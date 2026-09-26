// Package driver 提供 token 存储后端的具体实现。
//
// 文件名 = 驱动名：database.go / redis.go / ...；
// 每个文件只放一个驱动的实现，由 internal/infra/token 包的工厂函数选用。
package driver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"

	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// Sentinel errors：供 token 包通过 re-export 对外暴露（避免 driver → token 反向依赖）。
//
// 调用方在使用 token.Get() 时仍写 errors.Is(err, token.ErrXxx)；
// 这里定义的是底层真实变量，token 包用同名 var 别名引用。
var (
	// ErrTokenNotFound token 不存在（哈希后查不到任何记录）。
	ErrTokenNotFound = errors.New("token: not found")

	// ErrTokenInvalid token 入参非法（空字符串等）。
	ErrTokenInvalid = errors.New("token: invalid")
)

// DatabaseDriver 是基于 GORM 的 token 存储实现。
//
// 持有的 base *gorm.DB 是 database.Init() 注册了 dbresolver 之后的全局实例，
// 每次操作由 dbresolver 自动按读写类型路由：
//   - Create / Update / Delete / 事务 → 写库
//   - Find / First / Take / Raw → 读副本（若启用）
//
// 这里不显式调用 dbresolver.Write()，让 token 校验的"读"享受读副本、
// 让 token 创建 / 删除的"写"自动落到写库。
type DatabaseDriver struct {
	base *gorm.DB
}

// NewDatabase 返回一个基于 *gorm.DB 的 token 驱动。
// 由 internal/infra/token.Init() 在 database.Init() 之后调用。
func NewDatabase(db *gorm.DB) *DatabaseDriver {
	return &DatabaseDriver{base: db}
}

// HashToken 把明文 token 哈希成 SHA256 hex 字符串（64 字符）。
//
// 选用 SHA256 而非 bcrypt / scrypt：token 本身是高熵随机串（不是用户口令），
// 不需要慢哈希抗暴力破解；SHA256 已经足够防止明文泄露后被反查 / 重放。
//
// 暴露为包级函数是为了让 token 包内调用方（如测试 / 业务侧）也能复用同一实现，
// 避免各 driver 各自实现导致行为不一致。
func HashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// Create 把 t 写入 tokens 表。
//
// 步骤：
//  1. 把 t.Token 明文哈希成 SHA256 hex 再覆盖字段；
//  2. base.WithContext 让 ctx 取消时 GORM 自动中止查询；
//  3. Create 触发 dbresolver 写库路由。
func (d *DatabaseDriver) Create(ctx context.Context, t *model.Token) error {
	if t == nil {
		return ErrTokenInvalid
	}
	t.Token = HashToken(t.Token)

	return d.base.WithContext(ctx).Create(t).Error
}

// Get 按明文 token 查一条；找不到返回 ErrTokenNotFound（包装 gorm.ErrRecordNotFound）。
func (d *DatabaseDriver) Get(ctx context.Context, rawToken string) (*model.Token, error) {
	if rawToken == "" {
		return nil, ErrTokenInvalid
	}
	hashed := HashToken(rawToken)

	var t model.Token
	err := d.base.WithContext(ctx).
		Where("token = ?", hashed).
		First(&t).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, err
	}
	return &t, nil
}

// Delete 按明文 token 软删一条；不存在 no-op。
//
// GORM 默认走软删除路径，自动写 deleted_at；Find / First / Where
// 默认会加 deleted_at IS NULL 过滤，所以删除后 Get 自然查不到。
func (d *DatabaseDriver) Delete(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return ErrTokenInvalid
	}
	hashed := HashToken(rawToken)

	return d.base.WithContext(ctx).
		Where("token = ?", hashed).
		Delete(&model.Token{}).Error
}

// Clear 删除指定用户在指定类型下的所有 token；用于"挤下线"或"重置登录态"。
//
// 软删而非硬删：与全表删除策略一致，保留审计痕迹；命中 idx_user_type(user_id, type) 复合索引。
func (d *DatabaseDriver) Clear(ctx context.Context, userID int64, tokenType string) error {
	if userID <= 0 || tokenType == "" {
		return ErrTokenInvalid
	}

	return d.base.WithContext(ctx).
		Where("user_id = ? AND type = ?", userID, tokenType).
		Delete(&model.Token{}).Error
}
