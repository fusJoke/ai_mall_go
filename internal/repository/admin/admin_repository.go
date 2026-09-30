// Package admin 存放管理员域相关的数据访问实现，按 CLAUDE.md 分层归到
// `internal/repository/admin/` 子目录。每个文件一个 Repository，
// 只做 DB → 内存 / 内存 → DB 的搬运，不掺业务。
//
// 当前文件：admin_repository.go —— 提供 Admin 模型的 GetByID 查询。
package admin

import (
	"errors"

	"gorm.io/gorm"

	"ai-go-mall/internal/model"
)

// AdminRepository 定义管理员域的最小查询接口。
//
// 设计原则（与 design D5 / D9 一致）：
//   - 接口方法只做"按主键 / 按字段"取行，不掺业务过滤（status、deleted_at
//     之类的业务字段过滤由 Service 层 / Manager 负责）；
//   - GORM 自动软删：底层调用 gorm.DB.First/Find 时 GORM 会自动加
//     `deleted_at IS NULL` 过滤，因此本接口不需要显式提供"含软删"变体；
//   - 多态用接口 + GORM 实现，未来替换 mock / 其他 driver 时调用方零改动。
type AdminRepository interface {
	// GetByID 按主键查管理员行；未命中时返回 (nil, nil)（区别于 DB 错误）。
	//
	// 调用方拿到 (admin, nil) 后需自行校验 admin != nil 与 admin.Status == 1。
	GetByID(uid uint) (*model.Admin, error)
}

// gormAdminRepository 是基于 *gorm.DB 的 AdminRepository 实现。
type gormAdminRepository struct {
	db *gorm.DB
}

// NewAdminRepository 构造一个 AdminRepository。db 由调用方注入（通常
// `database.Get()`），便于在测试中替换为 mock 或事务内的 DB。
func NewAdminRepository(db *gorm.DB) AdminRepository {
	return &gormAdminRepository{db: db}
}

// GetByID 实现 AdminRepository.GetByID。
//
// 行为细节：
//   - 软删除的行（gorm.DeletedAt != 零值）由 GORM 自动从结果中过滤，调用方拿不到；
//   - 不存在的行返回 (nil, nil)，不当作错误；
//   - 底层 DB 错误（如连接断开）原样返回。
func (r *gormAdminRepository) GetByID(uid uint) (*model.Admin, error) {
	var admin model.Admin
	err := r.db.First(&admin, uid).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &admin, nil
}
