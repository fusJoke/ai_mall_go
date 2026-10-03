// Package draw_check — user_active_check.go 实现「用户账号是否可用」校验节点。
//
// 校验内容（spec Requirement "Draw transaction atomicity" 前置校验 #3）：
//
//	mall_users.status = 1
//
// 失败出口：chain.ErrUserNotAvailable（handler 映射 HTTP 403 draw.user_not_available）。
//
// 设计要点：
//   - 单一职责，只读 users 表一次；
//   - 「不存在」与「已禁用」用同一个 sentinel（与现有 draw.go 行为一致，
//     防止暴露存在性）；
//   - 防御性兜底：UserRepository 返回值即便不报 gorm.ErrRecordNotFound（如
//     soft delete 后的 nil 行）也走 ErrUserNotAvailable。
package draw_check

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	userRepo "ai-go-mall/internal/repository/user"
)

// userActiveCheck 校验用户账号可用（status=1）。
type userActiveCheck struct {
	users userRepo.UserRepository
}

// NewUserActiveCheck 构造 userActiveCheck Validator。
func NewUserActiveCheck(users userRepo.UserRepository) chain.Validator {
	return &userActiveCheck{users: users}
}

// Name 实现 chain.Validator。
func (c *userActiveCheck) Name() string { return "user_active" }

// Validate 读 users 表，校验账号可用。
//
// 通过：return nil。
// 拒绝：return chain.ErrUserNotAvailable（wrap 由 Chain.Validate 完成）。
func (c *userActiveCheck) Validate(ctx *gin.Context, input *chain.DrawInput) error {
	if input.UserID <= 0 {
		return chain.ErrUserNotAvailable
	}
	u, err := c.users.GetByID(ctx, input.UserID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return chain.ErrUserNotAvailable
		}
		return err
	}
	if u == nil || u.Status != 1 {
		return chain.ErrUserNotAvailable
	}
	return nil
}

// 编译期断言。
var _ chain.Validator = (*userActiveCheck)(nil)
