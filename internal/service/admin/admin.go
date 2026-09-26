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

	"ai-go-mall/internal/model"
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/service"
)

// tokenIssuer 是 service 层对 token 写入能力的最小抽象。
//
// *token.Manager 满足此接口；测试可注入 mock。抽接口成本极低，
// 但能让 service 层测试不依赖真实 token.Manager（也就无需真实 DB）。
type tokenIssuer interface {
	Create(ctx context.Context, t *model.Token) error
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

// 通用错误：用户名 / 密码不匹配 / 账号被禁用。统一文案避免泄露用户名是否存在。
var (
	ErrInvalidCredentials = errors.New("admin: invalid username or password")
	ErrAccountDisabled    = errors.New("admin: account disabled")
)

// Service 是管理员实体的业务接口。
//
// 调用方（handler / 业务编排）只持有接口；具体实现由 NewService 返回。
type Service interface {
	service.CRUDService[model.Admin]

	// Login 校验用户名 + 密码；成功返回管理员记录 + 明文 token（Password 字段是哈希，调用方不应原样返回）。
	//
	// 业务规则：
	//   - 用户名 / 密码任意一项错误统一返回 ErrInvalidCredentials，不泄露用户名是否存在。
	//   - 账号 Status != 1 直接返回 ErrAccountDisabled。
	//   - 登录成功：LoginFailure 清零、LastLoginAt = now、LastLoginIp = c.ClientIP()，
	//     并签发 token（remember=true → TokenTTLRemember，否则 TokenTTLShort）。
	//   - 登录失败：LoginFailure 自增并落库；不签发 token。
	//   - token 落库失败：返回 error（admin 状态更新已落库，前端应提示重试）。
	Login(c *gin.Context, username, password string, remember bool) (*model.Admin, string, error)
}

// baseService 是 Service 的默认实现。
//
// 嵌入 service.CRUDService[model.Admin] 转发通用 CRUD；
// 额外持有 repo（adminRepo.Repository）以拿到 GetByUsername，
// 与 tm（tokenIssuer）以签发登录 token。
type baseService struct {
	service.CRUDService[model.Admin]
	repo adminRepo.Repository
	tm   tokenIssuer
}

// NewService 接收 admin repo 与 token 签发器，返回 Service 接口。
func NewService(repo adminRepo.Repository, tm tokenIssuer) Service {
	return &baseService{
		CRUDService: service.NewBaseCRUDService[model.Admin](repo),
		repo:        repo,
		tm:          tm,
	}
}

// Login 实现见 Service 注释。
//
// 错误顺序：先按用户名查 → 不存在即无效凭据（同时不递增计数）→ 比对哈希 →
// 不匹配递增 LoginFailure 并落库 → 检查状态 → 通过则重置 + 更新最后登录信息 + 签发 token。
func (s *baseService) Login(c *gin.Context, username, password string, remember bool) (*model.Admin, string, error) {
	adm, err := s.repo.GetByUsername(c, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrInvalidCredentials
		}
		return nil, "", err
	}

	// 密码错误：递增失败计数后返回。落库失败不阻塞登录失败响应，避免抖动。
	if bcryptErr := bcrypt.CompareHashAndPassword([]byte(adm.Password), []byte(password)); bcryptErr != nil {
		adm.LoginFailure++
		_ = s.Update(c, adm)
		return nil, "", ErrInvalidCredentials
	}

	// 状态校验必须在密码通过后做，避免禁用账号被用来探测用户名是否存在。
	if adm.Status != 1 {
		return nil, "", ErrAccountDisabled
	}

	// 登录成功：清零失败次数 + 记录最后登录。
	adm.LoginFailure = 0
	now := time.Now()
	adm.LastLoginAt = &now
	adm.LastLoginIp = c.ClientIP()
	if err := s.Update(c, adm); err != nil {
		return nil, "", err
	}

	// 签发 token：uuid v7 明文 + 根据 remember 选 TTL。
	// 用 c.Request.Context() 而非 c，让 GORM 在 ctx 取消时中止查询。
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

// 编译期断言：baseService 必须实现 Service。
var _ Service = (*baseService)(nil)