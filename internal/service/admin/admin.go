// Package admin 是管理员实体的业务编排层。
//
// 通用 CRUD 走 repository.CRUDService 转发；业务专属能力（Login）
// 由本包自行实现，叠加：失败计数维护、最后登录信息更新、密码哈希校验（bcrypt）
// + 登录成功签发 token。
package admin

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ai-go-mall/internal/middleware"
	"ai-go-mall/internal/model"
	adminRepo "ai-go-mall/internal/repository/admin"
	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/service"
)

// tokenIssuer 是 service 层对 token 写入能力的最小抽象。
//
// *token.Manager 满足此接口；测试可注入 mock。抽接口成本极低，
// 但能让 service 层测试不依赖真实 token.Manager（也就无需真实 DB）。
type tokenIssuer interface {
	Create(ctx context.Context, t *model.Token) error

	// Delete 软删除 rawToken 对应的记录；幂等 —— 不存在 / 已删除 / 已过期一律返回 nil。
	// *token.Manager.Delete 已在 internal/infra/token/token.go:131-137 实现。
	Delete(ctx context.Context, rawToken string) error

	// Clear 删除指定用户在指定 type 下的全部 token；用于「账号锁定/禁用时吊销既有会话」。
	// *token.Manager.Clear 已在 internal/infra/token/token.go:139-145 实现。
	Clear(ctx context.Context, userID int64, tokenType string) error
}

// captchaVerifier 是 service 层对点选验证码「消费型 verify」的最小抽象。
//
// *infra/captcha.Manager 满足此接口；测试可注入 mock。
// 设计为最小接口（只暴露 VerifyClick），是为了避免 service/admin 对
// infra/captcha 包的过度耦合。
//
// `verifyCaptcha` 入参 (key, points, w, h) 与业务一致；
// deleteOnSuccess=true 表示「消费型」（登录链路二次校验）。
type captchaVerifier interface {
	VerifyClick(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error
}

// token 有效期常量（管理员登录场景）。
//
// 未来若要可配置化，只需把这里改成读 config.Get().Token.TTL.*，
// 调用点（Login 内 ttl 选择）不变。
const (
	// TokenTTLRemember 管理员"记住我"登录 token 有效期：30 天。
	TokenTTLRemember = 30 * 24 * time.Hour

	// TokenTTLShort 管理员默认登录 token 有效期：3 天。
	TokenTTLShort = 3 * 24 * time.Hour
)

// token 类型标识：登录签发的 token 全部归为 "admin"。
//
// 未来如果加 "refresh" / "api" 等类型时再抽 enum 或挪到 model 包。
const tokenTypeAdmin = "admin"

// MaxLoginFailure 管理员连续登录失败次数阈值：超过即把 Status 置 0（禁用）。
//
// 触发逻辑见 Login 的密码错误分支。
//
// 后续若要可配置化，只需把这里改成读 config.Get().Admin.MaxLoginFailure。
const MaxLoginFailure = 5

// 通用错误：用户名 / 密码不匹配 / 账号被禁用 / 验证码错误。统一文案避免泄露用户名是否存在。
var (
	ErrInvalidCredentials = errors.New("admin: invalid username or password")
	ErrAccountDisabled    = errors.New("admin: account disabled")
	ErrInvalidCaptcha     = errors.New("admin: invalid captcha")
)

// 管理页专用错误。
//
// handler 把 ErrPasswordTooShort 映射成 4xx admin.change_password.password_too_short
// 把 ErrSelfProtection 映射成 403 admin.toggle_status.self_protection。
var (
	ErrPasswordTooShort = errors.New("admin: password too short")
	ErrSelfProtection   = errors.New("admin: refusing self-targeting operation")
)

// 密码 / 用户名长度下限常量。
//
// 故意不在配置层暴露：管理页是内部工具，最小校验就够；如要外部化请先讨论配置结构。
const (
	MinPasswordLength = 8
	MinUsernameLength = 3
)

// Service 是管理员实体的业务接口。
//
// 调用方（handler / 业务编排）只持有接口；具体实现由 NewService 返回。
type Service interface {
	service.CRUDService[model.Admin]

	// Login 校验「点选验证码 + 用户名 + 密码」；成功返回管理员记录 + 明文 token
	// （Password 字段是哈希，调用方不应原样返回）。
	//
	// 业务规则（按顺序门控，任一环节失败立即终止）：
	//  1. captcha 二次校验通过（consume 语义，deleteOnSuccess=true）。
	//  2. 用户名 / 密码任意一项错误统一返回 ErrInvalidCredentials，不泄露用户名是否存在。
	//  3. 账号 Status != 1 直接返回 ErrAccountDisabled。
	//  4. 登录成功：LoginFailure 清零、LastLoginAt = now、LastLoginIp = c.ClientIP()，
	//     并签发 token（remember=true → TokenTTLRemember，否则 TokenTTLShort）。
	//  5. token 落库失败：返回 error（admin 状态更新已落库，前端应提示重试）。
	Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*model.Admin, string, error)

	// Logout 软删除当前调用方持有的 token；幂等 —— token 缺失 / 已过期 / 不存在一律返回 nil。
	// 由 tokenIssuer.Delete 保证幂等性，service 层不做额外包装。
	Logout(ctx context.Context, rawToken string) error

	// ChangePassword 改密：bcrypt 哈希后写库；成功后吊销该 admin 全部 admin token。
	// newPassword 长度 < MinPasswordLength → ErrPasswordTooShort；repo 错误原样返回。
	// token 吊销失败仅 log warn，不阻塞主流程。
	ChangePassword(c *gin.Context, id uint, newPassword string) error

	// ToggleStatus 切换 Status：1→0 时调 tm.Clear 吊销该 admin token；0→1 不吊销。
	// self-protection：目标 id == 当前 admin.ID → ErrSelfProtection（admin 不能禁用自己的账号）。
	// 不存在 → gorm.ErrRecordNotFound。
	ToggleStatus(c *gin.Context, id uint) error

	// Unlock 重置 LoginFailure=0 + Status=1；不解锁密码、不吊销 token
	// （admin 解锁是「让用户继续登录」，不是「逼他下线」，与 buildadmin 行为一致）。
	Unlock(c *gin.Context, id uint) error

	// BatchDelete 软删一组 id；调用前剔除 self-id，返回 (deleted, skippedSelf, err)。
	// 空 ids → (0, 0, nil)；只剩 self → (0, len(ids), nil)；repo 错透传。
	BatchDelete(c *gin.Context, ids []uint) (deleted int, skippedSelf int, err error)
}

// baseService 是 Service 的默认实现。
//
// 嵌入 service.CRUDService[model.Admin] 转发通用 CRUD；
// 额外持有 repo（adminRepo.Repository）以拿到 GetByUsername，
// tm（tokenIssuer）以签发登录 token，cv（captchaVerifier）以做点选验证码二次校验。
type baseService struct {
	service.CRUDService[model.Admin]
	repo adminRepo.Repository
	tm   tokenIssuer
	cv   captchaVerifier
}

// NewService 接收 admin repo / token 签发器 / captcha 校验器，返回 Service 接口。
func NewService(repo adminRepo.Repository, tm tokenIssuer, cv captchaVerifier) Service {
	return &baseService{
		CRUDService: service.NewBaseCRUDService[model.Admin](repo),
		repo:        repo,
		tm:          tm,
		cv:          cv,
	}
}

// Login 实现见 Service 注释。
//
// 顺序：captcha 二次校验 → 用户名查询 → bcrypt → 状态 → token。
// captcha 失败立即返回 ErrInvalidCaptcha，不进入密码分支（防止脚本试探密码计数）。
func (s *baseService) Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*model.Admin, string, error) {
	// 1) 验证码二次校验（consume 语义，删除成功路径上的 key）。
	if err := s.cv.VerifyClick(c.Request.Context(), &captchaInfra.VerifyReq{
		Key:    captchaKey,
		Points: points,
		W:      captchaInfra.ImageWidth,
		H:      captchaInfra.ImageHeight,
	}, true); err != nil {
		return nil, "", ErrInvalidCaptcha
	}

	// 2) 用户名查询。
	adm, err := s.repo.GetByUsername(c, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	// 3) 密码错误：递增失败计数。达到阈值就把 Status 置 0（锁定），并吊销该账号
	// 既有 token —— 否则攻击者在被锁前签发的合法 token 在剩余 TTL 内仍能访问受保护资源。
	// 落库 / Clear 失败不阻塞登录失败响应（防御动作，非关键路径）。
	//
	// 落库走定向列更新（UpdateLoginFailure），禁止走 s.Update：此处模型带出的是
	// 哈希形态的 Password，整行更新会被 hashPasswordIfNeeded 二次哈希，
	// 导致正确密码永久失效（P0，见 openspec/changes/fix-admin-login-password-rehash）。
	if bcryptErr := bcrypt.CompareHashAndPassword([]byte(adm.Password), []byte(password)); bcryptErr != nil {
		failure := adm.LoginFailure + 1
		locked := failure >= MaxLoginFailure
		if locked {
			_ = s.tm.Clear(c.Request.Context(), adm.ID, tokenTypeAdmin)
		}
		_ = s.repo.UpdateLoginFailure(c, uint(adm.ID), failure, locked)
		return nil, "", ErrInvalidCredentials
	}

	// 4) 状态校验必须在密码通过后做，避免禁用账号被用来探测用户名是否存在。
	if adm.Status != 1 {
		return nil, "", ErrAccountDisabled
	}

	// 5) 登录成功：清零失败次数 + 记录最后登录。
	// 内存模型同步赋值仅供登录响应体展示；DB 落库走定向列更新（UpdateLoginSuccess），
	// 原因同分支 3 —— 禁止把带哈希 Password 的模型交给 s.Update。
	now := time.Now()
	adm.LoginFailure = 0
	adm.LastLoginAt = &now
	adm.LastLoginIp = c.ClientIP()
	if err := s.repo.UpdateLoginSuccess(c, uint(adm.ID), adm.LastLoginIp, now); err != nil {
		return nil, "", err
	}

	// 签发 token：uuid v7 明文 + 根据 remember 选 TTL。
	uid, err := uuid.NewV7()
	if err != nil {
		return nil, "", err
	}
	rawToken := uid.String()
	ttl := TokenTTLShort
	if remember {
		ttl = TokenTTLRemember
	}
	tok := &model.Token{
		Token:     rawToken,
		Type:      tokenTypeAdmin,
		UserID:    adm.ID,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.tm.Create(c.Request.Context(), tok); err != nil {
		return nil, "", err
	}

	return adm, rawToken, nil
}

// Logout 实现见 Service 注释。直接转发到 tokenIssuer.Delete —— 幂等性由 token infra 保证。
func (s *baseService) Logout(ctx context.Context, rawToken string) error {
	return s.tm.Delete(ctx, rawToken)
}

// --- 管理页专属方法 ---

// Create 在转发到嵌入 CRUDService.Create 之前做密码哈希。
//
// admin 业务规则：Password 字段进入 service 必须已被调用方填上明文；空值视作编程错误，
// 由 nil 检查 + ErrPasswordTooShort 兜底。用户名查重由 GORM 唯一索引兜底，
// 不在 service 层重复 GetByUsername。
func (s *baseService) Create(c *gin.Context, entity *model.Admin) error {
	if entity == nil {
		return errors.New("service: nil entity")
	}
	if err := s.hashPasswordIfNeeded(c, entity, false); err != nil {
		return err
	}
	return s.CRUDService.Create(c, entity)
}

// Update 在转发到嵌入 CRUDService.Update 之前按"非空则改密"语义处理 Password 字段。
//
// 前端部分更新：省略 password 字段 → entity.Password == "" → 不动原密码；
// 显式给新密码 → 走 bcrypt。
func (s *baseService) Update(c *gin.Context, entity *model.Admin) error {
	if entity == nil {
		return errors.New("service: nil entity")
	}
	if err := s.hashPasswordIfNeeded(c, entity, true); err != nil {
		return err
	}
	return s.CRUDService.Update(c, entity)
}

// hashPasswordIfNeeded 按 Update/Create 语义处理 Password：
//
//   - isUpdate=true && Password == "" → 直接 return nil，保留 DB 原值；
//   - 其他情况 → 长度校验 < MinPasswordLength 返 ErrPasswordTooShort；
//     bcrypt.DefaultCost 哈希后回写 entity.Password。
//
// 故意不读 DB 当前值再合并：service 层语义是"前端给什么就存什么"，空 password 在
// Update 路径下语义明确 = "不改"；Create 路径下空 password 等同"漏字段"，应报错。
func (s *baseService) hashPasswordIfNeeded(_ *gin.Context, entity *model.Admin, isUpdate bool) error {
	if isUpdate && entity.Password == "" {
		return nil
	}
	if len(entity.Password) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(entity.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	entity.Password = string(hashed)
	return nil
}

// ChangePassword 见 Service 接口。
//
// 流程：长度校验 → bcrypt → repo.UpdatePassword → tm.Clear（失败 log warn 不阻塞）。
func (s *baseService) ChangePassword(c *gin.Context, id uint, newPassword string) error {
	if len(newPassword) < MinPasswordLength {
		return ErrPasswordTooShort
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.repo.UpdatePassword(c, id, string(hashed)); err != nil {
		return err
	}
	// 改密后强制吊销该 admin 全部 admin token，让用户用新密码重新登录。
	if err := s.tm.Clear(c.Request.Context(), int64(id), tokenTypeAdmin); err != nil {
		// 防御动作：吊销失败不阻塞「密码已改」这个事实。
		// 生产环境可考虑接 zap / logrus 写 warn 日志；service 包当前没有 logger 依赖。
		_ = err
	}
	return nil
}

// ToggleStatus 见 Service 接口。
//
// self-protection：当前 admin 不能禁用自己的账号（防止 super admin 把所有 admin 一起锁掉）。
//
// 1→0 切到禁用时同步吊销该 admin 的 token；0→1 不吊销（admin 解禁用户是为了让 ta 继续用）。
func (s *baseService) ToggleStatus(c *gin.Context, id uint) error {
	if self := middleware.AdminFromContext(c); self != nil && uint(self.ID) == id {
		return ErrSelfProtection
	}
	adm, err := s.repo.GetByID(c, int64(id))
	if err != nil {
		return err
	}
	if adm == nil {
		return gorm.ErrRecordNotFound
	}

	var newStatus int8
	if adm.Status == 1 {
		newStatus = 0
	} else {
		newStatus = 1
	}
	if err := s.repo.UpdateStatus(c, id, newStatus); err != nil {
		return err
	}
	// 1→0 才吊销 token；0→1 不动。
	if adm.Status == 1 {
		if err := s.tm.Clear(c.Request.Context(), int64(id), tokenTypeAdmin); err != nil {
			_ = err
		}
	}
	return nil
}

// Unlock 见 Service 接口。直接调 repo.ResetLoginFailure —— 一行的事，不读 self / 不清 token。
func (s *baseService) Unlock(c *gin.Context, id uint) error {
	return s.repo.ResetLoginFailure(c, id)
}

// BatchDelete 见 Service 接口。
//
// 流程：剔除 self-id → 调 repo.DeleteBatch（空切片短路）→ 返回计数。
// 不开事务：删除是幂等的，软删即便重复执行也只动 deleted_at。
func (s *baseService) BatchDelete(c *gin.Context, ids []uint) (int, int, error) {
	self := middleware.AdminFromContext(c)

	filtered := make([]uint, 0, len(ids))
	var skippedSelf int
	for _, id := range ids {
		if self != nil && id == uint(self.ID) {
			skippedSelf++
			continue
		}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 {
		return 0, skippedSelf, nil
	}
	if err := s.repo.DeleteBatch(c, filtered); err != nil {
		return 0, skippedSelf, err
	}
	return len(filtered), skippedSelf, nil
}

// 编译期断言：baseService 必须实现 Service。
var _ Service = (*baseService)(nil)