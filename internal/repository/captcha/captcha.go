// Package captcha 实现 captcha 模型的数据访问。
//
// 设计要点（与 repository 基类的差异）：
//   - 全部方法接 context.Context 而不是 *gin.Context：
//     1. cleanup goroutine 没有 gin.Context，捕获客户端断开语义靠 context；
//     2. context.Context 是 Go 通用约定，新包不必迁就老接口。
//   - 全部走写库：Captcha 表的写后立刻可能读（校验场景），
//     走读副本会有秒级延迟，反而把"刚生成就拿不到 key"的概率拉高。
//   - DeleteByKey 走硬删：model.Captcha 没有 DeletedAt 字段，
//     GORM 默认行为即 DELETE FROM。
//
// 主键约定：model.Captcha.Key 是 MySQL 保留字，GORM 自动反引号转义主键，
// 但手写 Where("key = ?") 仍需用反引号。本包内全部用 Where("`key` = ?", ...)。
package captcha

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model"
)

// ErrNotFound captcha 不存在（key 错误或已被删除/过期清理）。
//
// 业务上 404 与 "已被验证后删除" 应同等对待，不应区分。
var ErrNotFound = errors.New("captcha: not found")

// Repository 是 captcha 表的数据访问接口。
type Repository interface {
	// Create 写入一条 captcha 记录。cpt.Key 必须由调用方生成（建议 uuid）。
	Create(ctx context.Context, cpt *model.Captcha) error

	// GetByKey 按主键读取一条记录；不存在返回 ErrNotFound。
	GetByKey(ctx context.Context, key string) (*model.Captcha, error)

	// DeleteByKey 按主键硬删一条记录；不存在不报错（no-op）。
	DeleteByKey(ctx context.Context, key string) error

	// DeleteExpired 删除 expires_at < now 的全部记录，返回删除行数。
	// 用于 cleanup goroutine 定期清理。
	DeleteExpired(ctx context.Context, now time.Time) (int64, error)
}

// repository 是默认实现，通过闭包延迟取 db，避免 Init 顺序问题。
type repository struct {
	db func() *gorm.DB
}

// New 返回默认实现的 Repository 接口。
//
// db 由闭包提供：初始化时通过 database.Get() 拿不到 db（test 场景）
// 也能通过覆盖闭包注入 fake db，便于测试。
func New() Repository {
	return &repository{db: database.Get}
}

// newWithDB 允许测试注入自定义 db 取值器。
func newWithDB(getDB func() *gorm.DB) Repository {
	return &repository{db: getDB}
}

func (r *repository) Create(ctx context.Context, cpt *model.Captcha) error {
	if cpt == nil {
		return errors.New("captcha repository: nil entity")
	}
	return r.db().WithContext(ctx).Create(cpt).Error
}

func (r *repository) GetByKey(ctx context.Context, key string) (*model.Captcha, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	var cpt model.Captcha
	// key 是 MySQL 保留字，必须反引号；GORM 对主键自动转义，但显式写更安全。
	err := r.db().WithContext(ctx).Where("`key` = ?", key).First(&cpt).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &cpt, nil
}

func (r *repository) DeleteByKey(ctx context.Context, key string) error {
	if key == "" {
		return nil
	}
	// Unscoped 防止 GORM 因软删除约定拦截；本表本就没有 DeletedAt 字段，这里是双保险。
	return r.db().WithContext(ctx).Unscoped().Where("`key` = ?", key).Delete(&model.Captcha{}).Error
}

func (r *repository) DeleteExpired(ctx context.Context, now time.Time) (int64, error) {
	res := r.db().WithContext(ctx).Unscoped().
		Where("expires_at < ?", now).
		Delete(&model.Captcha{})
	return res.RowsAffected, res.Error
}
