// Package repository 是数据访问层，只负责 DB ↔ 内存 的搬运，不掺业务逻辑。
//
// 当前文件仅放跨 repository 共用的小工具；具体 repository 后续按业务实体划分。
package repository

import (
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/database"
)

// DB 取出当前请求 ctx 作用域内的 *gorm.DB，由 DBMiddleware 注入。
//
// 缺失时 panic（视为编程错误：未注册中间件）。典型用法：
//
//	func (r *userRepo) FindByID(c *gin.Context, id int64) (*model.User, error) {
//	    return repository.DB(c).First(&model.User{}, id).Error()
//	}
func DB(c *gin.Context) *gorm.DB {
	return database.FromContext(c)
}
