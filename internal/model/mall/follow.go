package mall

import (
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type MallUserFollowSupplier struct {
	ID         int64          `json:"id" gorm:"comment:ID;primaryKey;autoIncrement"`
	UserID     int64          `json:"user_id" gorm:"comment:user_id;not null;uniqueIndex:uk_follow_user_supplier,priority:1"`
	SupplierID int64          `json:"supplier_id" gorm:"comment:supplier_id;not null;uniqueIndex:uk_follow_user_supplier,priority:2;index:idx_follow_supplier"`
	UpdatedAt  time.Time      `json:"updated_at" gorm:"comment:updated_at;not null;autoUpdateTime"`
	CreatedAt  time.Time      `json:"created_at" gorm:"comment:created_at;not null;autoCreateTime"`
	DeletedAt  gorm.DeletedAt `json:"deleted_at" gorm:"comment:deleted_at;index"`
}

func (MallUserFollowSupplier) TableName(namer schema.Namer) string {
	return namer.TableName("mall_user_follow_supplier")
}