// Package supplier 是 B 端供应商（mall_supplier_users 表）的业务编排层。
//
// 通用 CRUD 走 service.CRUDService[mall.MallSupplierUser] 转发；业务专属能力（Login）
// 由本包自行实现：密码哈希校验（bcrypt）+ 登录成功签发 token（type="supplier"）。
//
// 与 service/user 的差异（按 mall_supplier_users 表 schema 决定）：
//   - 没有 LoginFailure 列 → 不做「连续失败 N 次自动禁用」的递增锁定；
//     admin 禁用账号直接走 UpdateStatus 即可。
//   - 不抽 tokenIssuer / captchaVerifier 的导出包级 alias：直接复用 service/user
//     的接口定义（同一进程内 token / captcha infra 一致；导出它们仅是命名冗余）。
package supplier

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
	"ai-go-mall/internal/model/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	"ai-go-mall/internal/service"
)

// tokenIssuer 是 service 层对 token 写入能力的最小抽象。
//
// 与 service/user 同一接口定义；infra/token.Manager 满足之。
type tokenIssuer interface {
	Create(ctx context.Context, t *model.Token) error
	Delete(ctx context.Context, rawToken string) error
	Clear(ctx context.Context, userID int64, tokenType string) error
}

// captchaVerifier 与 service/user 同一接口定义。
type captchaVerifier interface {
	VerifyClick(ctx context.Context, req *captchaInfra.VerifyReq, deleteOnSuccess bool) error
}

// token 有效期常量（B 端供应商登录场景，与 user / admin 对齐）。
//
// 30 天（记住我）/ 3 天（默认）。供应商后台不是高频工具，无需短 TTL。
const (
	TokenTTLRemember = 30 * 24 * time.Hour
	TokenTTLShort    = 3 * 24 * time.Hour
)

// token 类型标识：B 端登录签发的 token 全部归为 "supplier"。
//
// 与 user（"user"）/ admin（"admin"）形成三段式，方便 token 表按 type 查路由、按 type 吊销。
const tokenTypeSupplier = "supplier"

// 通用错误：用户名 / 密码不匹配 / 账号被禁用 / 验证码错误。统一文案避免泄露用户名是否存在。
//
// 与 service/user / service/admin 的 error 文案保持同义（仅前缀不同），handler 可按业务上下文写 msg。
var (
	ErrInvalidCredentials = errors.New("supplier: invalid username or password")
	ErrAccountDisabled    = errors.New("supplier: account disabled")
	ErrInvalidCaptcha     = errors.New("supplier: invalid captcha")
)

// AuthService 是供应商账号登录业务接口。
//
// 独立于 supplier 包内其他 Service（如 product / promotion），业务边界清晰：auth 只管账号登录。
// CRUD 通用能力（Create / Update / Delete）由 supplier 包顶层 Service 暴露（后续 phase）。
type AuthService interface {
	// Login 校验「点选验证码 + 用户名 + 密码」；成功返回账号记录 + 明文 token。
	//
	// 业务规则（按顺序门控，任一环节失败立即终止）：
	//  1. captcha 二次校验通过（consume 语义，deleteOnSuccess=true）。
	//  2. 用户名 / 密码任意一项错误统一返回 ErrInvalidCredentials，不泄露用户名是否存在。
	//  3. 账号 Status != 1 直接返回 ErrAccountDisabled。
	//  4. 登录成功：LastLoginAt = now、LastLoginIp = c.ClientIP()，
	//     并签发 token（remember=true → TokenTTLRemember，否则 TokenTTLShort）。
	//  5. token 落库失败：返回 error（账号状态更新已落库，前端应提示重试）。
	//
	// 与 service/user 的差异：
	//   - 无 LoginFailure 列 → 不做「失败 N 次自动禁用」的递增锁定；
	//     禁用由 admin 走 UpdateStatus 单独控制。
	Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*mall.MallSupplierUser, string, error)

	// Logout 软删除当前调用方持有的 token；幂等 —— 由 tokenIssuer.Delete 保证。
	Logout(ctx context.Context, rawToken string) error
}

// baseAuthService 是 AuthService 的默认实现。
//
// 仅持有 supplierUser repo（其他 CRUD 走嵌入的 CRUDService）。
// sup 是供应商主体状态读取（最小接口，便于单测 mock）——
// 登录必须校验 mall_suppliers.status：账号启用但主体被 admin 禁用 → 拒绝
// （spec Scenario "Disabled supplier cannot login"，final review Important #3）。
type baseAuthService struct {
	service.CRUDService[mall.MallSupplierUser]
	repo supplierRepo.SupplierUserRepository
	sup  supplierStatusReader
	tm   tokenIssuer
	cv   captchaVerifier
}

// supplierStatusReader 是供应商主体状态读取的最小接口。
//
// 生产实现 = supplierRepo.SupplierRepository（其 GetByID 满足此签名）；
// 抽窄接口是为了单测不必实现整个仓储接口。
type supplierStatusReader interface {
	GetByID(c *gin.Context, id int64) (*mall.MallSupplier, error)
}

// NewAuthService 接收依赖，返回 AuthService 接口。
func NewAuthService(repo supplierRepo.SupplierUserRepository, sup supplierStatusReader, tm tokenIssuer, cv captchaVerifier) AuthService {
	return &baseAuthService{
		CRUDService: service.NewBaseCRUDService(repo),
		repo:        repo,
		sup:         sup,
		tm:          tm,
		cv:          cv,
	}
}

// Login 实现见 AuthService 注释。
//
// 顺序：captcha 二次校验 → 用户名查询 → bcrypt → 状态 → token。
// 与 service/user 形态一致，差异在失败计数与锁定逻辑（被省去，原因见包注释）。
func (s *baseAuthService) Login(c *gin.Context, username, password, captchaKey string, points []captchaInfra.Point, remember bool) (*mall.MallSupplierUser, string, error) {
	// 1) 验证码二次校验（consume 语义）。
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

	// 3) 密码错误：直接返回统一文案。无递增锁定（见包注释）。
	//
	// 落库走定向列更新（UpdateStatus 等）时禁止走 s.Update：此处模型带出的是哈希
	// 形态的 Password，整行更新会被二次哈希，导致正确密码永久失效（虽然失败分支
	// 这里不写库，但留个注释提醒后续扩展不要踩坑）。
	if bcryptErr := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); bcryptErr != nil {
		return nil, "", ErrInvalidCredentials
	}

	// 4) 状态校验必须在密码通过后做，避免禁用账号被用来探测用户名是否存在。
	if u.Status != 1 {
		return nil, "", ErrAccountDisabled
	}

	// 4.5) 供应商主体状态校验：账号启用但主体被 admin 禁用 → 拒绝。
	// 主体不存在（脏数据：supplier_id 悬空）同样按禁用处理，不暴露差异。
	if sup, err := s.sup.GetByID(c, u.SupplierID); err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", err
		}
		return nil, "", ErrAccountDisabled
	} else if sup == nil || sup.Status != mall.StatusActive {
		return nil, "", ErrAccountDisabled
	}

	// 5) 登录成功：记录最后登录。
	// 内存模型同步赋值仅供登录响应体展示；DB 落库走定向列更新（UpdateLoginSuccess），
	// 禁止把带哈希 Password 的模型交给 s.Update。
	now := time.Now()
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
		Type:      tokenTypeSupplier,
		UserID:    u.ID,
		ExpiresAt: now.Add(ttl),
	}
	if err := s.tm.Create(c.Request.Context(), tok); err != nil {
		return nil, "", err
	}

	return u, rawToken, nil
}

// Logout 实现见 AuthService 注释。直接转发到 tokenIssuer.Delete —— 幂等性由 token infra 保证。
func (s *baseAuthService) Logout(ctx context.Context, rawToken string) error {
	return s.tm.Delete(ctx, rawToken)
}

// 编译期断言：baseAuthService 必须实现 AuthService。
var _ AuthService = (*baseAuthService)(nil)
