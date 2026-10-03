// Package user 是 C 端会员（mall_users / users 表）的业务编排层。
//
// 通用 CRUD 走 service.CRUDService[model.User] 转发；业务专属能力（Login）
// 由本包自行实现，叠加：失败计数维护、最后登录信息更新、密码哈希校验（bcrypt）
// + 登录成功签发 token。
package user

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	"ai-go-mall/internal/model"
	userRepo "ai-go-mall/internal/repository/user"
	"ai-go-mall/internal/service"
)

// tokenIssuer 是 service 层对 token 写入能力的最小抽象。
//
// *token.Manager 满足此接口；测试可注入 mock。抽接口成本极低，
// 但能让 service 层测试不依赖真实 token.Manager（也就无需真实 DB）。
type tokenIssuer interface {
	Create(ctx context.Context, t *model.Token) error

	// Delete 软删除 rawToken 对应的记录；幂等 —— 不存在 / 已删除 / 已过期一律返回 nil。
	Delete(ctx context.Context, rawToken string) error

	// Clear 删除指定用户在指定 type 下的全部 token；用于「账号锁定/禁用时吊销既有会话」。
	Clear(ctx context.Context, userID int64, tokenType string) error
}

// captchaVerifier 是 service 层对点选验证码「消费型 verify」的最小抽象。
//
// *infra/captcha.Manager 满足此接口；测试可注入 mock。
// 设计为最小接口（只暴露 VerifyClick），是为了避免 service/user 对
// infra/captcha 包的过度耦合。
//
// `verifyCaptcha` 入参 (key, points, w, h) 与业务一致；
// deleteOnSuccess=true 表示「消费型」（登录链路二次校验）。
type captchaVerifier interface {
	VerifyClick(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error
}

// token 有效期常量（C 端会员登录场景）。
//
// 与 service/admin 保持一致：30 天（记住我）/ 3 天（默认）。
const (
	// TokenTTLRemember C 端"记住我"登录 token 有效期：30 天。
	TokenTTLRemember = 30 * 24 * time.Hour

	// TokenTTLShort C 端默认登录 token 有效期：3 天。
	TokenTTLShort = 3 * 24 * time.Hour
)

// token 类型标识：C 端登录签发的 token 全部归为 "user"。
const tokenTypeUser = "user"

// MaxLoginFailure 连续登录失败次数阈值：超过即把 Status 置 0（禁用）。
//
// 触发逻辑见 Login 的密码错误分支。
const MaxLoginFailure = 5

// 通用错误：用户名 / 密码不匹配 / 账号被禁用 / 验证码错误。统一文案避免泄露用户名是否存在。
var (
	ErrInvalidCredentials = errors.New("user: invalid username or password")
	ErrAccountDisabled    = errors.New("user: account disabled")
	ErrInvalidCaptcha     = errors.New("user: invalid captcha")
)

// Service 是 C 端会员实体的业务接口。
//
// 调用方（handler / 业务编排）只持有接口；具体实现由 NewService 返回。
type Service interface {
	service.CRUDService[model.User]

	// Login 校验「点选验证码 + 用户名 + 密码」；成功返回用户记录 + 明文 token
	// （Password 字段是哈希，调用方不应原样返回）。
	//
	// 业务规则（按顺序门控，任一环节失败立即终止）：
	//  1. captcha 二次校验通过（consume 语义，deleteOnSuccess=true）。
	//  2. 用户名 / 密码任意一项错误统一返回 ErrInvalidCredentials，不泄露用户名是否存在。
	//  3. 账号 Status != 1 直接返回 ErrAccountDisabled。
	//  4. 登录成功：LoginFailure 清零、LastLoginAt = now、LastLoginIp = c.ClientIP()，
	//     并签发 token（remember=true → TokenTTLRemember，否则 TokenTTLShort）。
	//  5. token 落库失败：返回 error（user 状态更新已落库，前端应提示重试）。
	Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*model.User, string, error)

	// Logout 软删除当前调用方持有的 token；幂等 —— token 缺失 / 已过期 / 不存在一律返回 nil。
	// 由 tokenIssuer.Delete 保证幂等性，service 层不做额外包装。
	Logout(ctx context.Context, rawToken string) error
}

// baseService 是 Service 的默认实现。
//
// 嵌入 service.CRUDService[model.User] 转发通用 CRUD；
// 额外持有 repo（userRepo.UserRepository）以拿到 GetByUsername / UpdateLoginFailure / UpdateLoginSuccess / UpdateStatus，
// tm（tokenIssuer）以签发登录 token，cv（captchaVerifier）以做点选验证码二次校验。
type baseService struct {
	service.CRUDService[model.User]
	repo userRepo.UserRepository
	tm   tokenIssuer
	cv   captchaVerifier
}

// NewService 接收 user repo / token 签发器 / captcha 校验器，返回 Service 接口。
func NewService(repo userRepo.UserRepository, tm tokenIssuer, cv captchaVerifier) Service {
	return &baseService{
		CRUDService: service.NewBaseCRUDService(repo),
		repo:        repo,
		tm:          tm,
		cv:          cv,
	}
}

// Login 实现见 Service 注释。
//
// 顺序：captcha 二次校验 → 用户名查询 → bcrypt → 状态 → token。
// captcha 失败立即返回 ErrInvalidCaptcha，不进入密码分支（防止脚本试探密码计数）。
func (s *baseService) Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*model.User, string, error) {
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
	u, err := s.repo.GetByUsername(c, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	// 3) 密码错误：原子自增失败计数（H1 修复）。达到阈值就把 Status 置 0（锁定），
	// 并吊销该账号既有 token —— 否则攻击者在被锁前签发的合法 token 在剩余 TTL
	// 内仍能访问受保护资源。
	// 落库 / Clear 失败不阻塞登录失败响应（防御动作，非关键路径）。
	//
	// 关于「原子性」（H1）：IncrementLoginFailure 把「读 u.LoginFailure → +1 → 写回」
	// 整体下推到 DB 的 `login_failure = login_failure + 1` + 同事务回读，避免两个
	// 并发错请求都从同一份 Go 层快照算 +1 后互相覆盖。
	//
	// 落库走定向列更新（IncrementLoginFailure / UpdateStatus），禁止走 s.Update：
	// 此处模型带出的是哈希形态的 Password，整行更新会被二次哈希，导致正确密码永久失效。
	if bcryptErr := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); bcryptErr != nil {
		newFailure, incrErr := s.repo.IncrementLoginFailure(c, u.ID)
		if incrErr != nil {
			// 防御动作：原子自增失败仅记日志，仍返回 ErrInvalidCredentials（不暴露内部错误）。
			_ = incrErr
		}
		locked := newFailure >= MaxLoginFailure
		if locked {
			_ = s.tm.Clear(c.Request.Context(), u.ID, tokenTypeUser)
			if statusErr := s.repo.UpdateStatus(c, u.ID, 0); statusErr != nil {
				_ = statusErr // 防御动作失败不阻塞登录失败响应。
			}
		}
		return nil, "", ErrInvalidCredentials
	}

	// 4) 状态校验必须在密码通过后做，避免禁用账号被用来探测用户名是否存在。
	if u.Status != 1 {
		return nil, "", ErrAccountDisabled
	}

	// 5) 登录成功：清零失败次数 + 记录最后登录。
	// 内存模型同步赋值仅供登录响应体展示；DB 落库走定向列更新（UpdateLoginSuccess），
	// 原因同分支 3 —— 禁止把带哈希 Password 的模型交给 s.Update。
	now := time.Now()
	u.LoginFailure = 0
	u.LastLoginAt = &now
	u.LastLoginIp = c.ClientIP()
	if err := s.repo.UpdateLoginSuccess(c, u.ID, u.LastLoginIp, now); err != nil {
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
		Type:      tokenTypeUser,
		UserID:    u.ID,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.tm.Create(c.Request.Context(), tok); err != nil {
		return nil, "", err
	}

	return u, rawToken, nil
}

// Logout 实现见 Service 注释。直接转发到 tokenIssuer.Delete —— 幂等性由 token infra 保证。
func (s *baseService) Logout(ctx context.Context, rawToken string) error {
	return s.tm.Delete(ctx, rawToken)
}

// 编译期断言：baseService 必须实现 Service。
var _ Service = (*baseService)(nil)
