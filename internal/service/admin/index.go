// Package admin 是管理员实体的业务编排层。
//
// 通用 CRUD 走 repository.CRUDService 转发；业务专属能力（Login）
// 由本包自行实现，叠加：失败计数维护、最后登录信息更新、密码哈希校验（bcrypt）。
package admin

import (
	"errors"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/service"
)

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

	// Login 校验用户名 + 密码；成功返回管理员记录（Password 字段是哈希，调用方不应原样返回）。
	//
	// 业务规则：
	//   - 用户名 / 密码任意一项错误统一返回 ErrInvalidCredentials，不泄露用户名是否存在。
	//   - 账号 Status != 1 直接返回 ErrAccountDisabled。
	//   - 登录成功：LoginFailure 清零、LastLoginAt = now、LastLoginIp = c.ClientIP()。
	//   - 登录失败：LoginFailure 自增并落库。
	Login(c *gin.Context, username, password string) (*model.Admin, error)
}

// baseService 是 Service 的默认实现。
//
// 嵌入 service.CRUDService[model.Admin] 转发通用 CRUD；
// 额外持有 repo（adminRepo.Repository）以拿到 GetByUsername。
type baseService struct {
	service.CRUDService[model.Admin]
	repo adminRepo.Repository
}

// NewService 接收 admin repo，返回 Service 接口。
func NewService(repo adminRepo.Repository) Service {
	return &baseService{
		CRUDService: service.NewBaseCRUDService[model.Admin](repo),
		repo:        repo,
	}
}

// Login 实现见 Service 注释。
//
// 错误顺序：先按用户名查 → 不存在即无效凭据（同时不递增计数）→ 比对哈希 →
// 不匹配递增 LoginFailure 并落库 → 检查状态 → 通过则重置 + 更新最后登录信息。
func (s *baseService) Login(c *gin.Context, username, password string) (*model.Admin, error) {
	adm, err := s.repo.GetByUsername(c, username)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	// 密码错误：递增失败计数后返回。落库失败不阻塞登录失败响应，避免抖动。
	if bcryptErr := bcrypt.CompareHashAndPassword([]byte(adm.Password), []byte(password)); bcryptErr != nil {
		adm.LoginFailure++
		_ = s.Update(c, adm)
		return nil, ErrInvalidCredentials
	}

	// 状态校验必须在密码通过后做，避免禁用账号被用来探测用户名是否存在。
	if adm.Status != 1 {
		return nil, ErrAccountDisabled
	}

	// 登录成功：清零失败次数 + 记录最后登录。
	adm.LoginFailure = 0
	now := time.Now()
	adm.LastLoginAt = &now
	adm.LastLoginIp = c.ClientIP()
	if err := s.Update(c, adm); err != nil {
		return nil, err
	}

	return adm, nil
}

// 编译期断言：baseService 必须实现 Service。
var _ Service = (*baseService)(nil)
