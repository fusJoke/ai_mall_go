// Package repository 是数据访问层，只负责 DB ↔ 内存 的搬运，不掺业务逻辑。
//
// 当前文件仅放跨 repository 共用的小工具；具体 repository 后续按业务实体划分。
package repository

import (
	"fmt"

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

// RunInTx 在事务内执行 fn。
//
// 行为：
//  1. 用 repository.DB(c) 拿到请求作用域 DB，开 tx；
//  2. 把 tx 写进 c.Copy() 出来的 txCtx 的 CtxKey；
//  3. fn 在 txCtx 上跑，期间任何 repository.DB(txCtx) 都拿到同一个 tx；
//  4. fn 返回 nil → Commit；返回 error → Rollback，并把 fn 错误上抛；
//  5. fn 内部 panic → Rollback + 重新 panic（保留调用栈）；
//  6. Commit/Rollback 本身出错 → wrap 到原 error 上返回（不静默吞）。
//
// 为什么用 c.Copy()：gin.Context.Set 会改内部 map，必须 copy 避免污染父 ctx；
// c.Copy() 是 gin 提供的官方安全拷贝。
//
// 用法（典型抽卡事务）：
//
//	err := repository.RunInTx(c, func(txCtx *gin.Context) error {
//	    items, err := s.poolRepo.ListItemsByPoolID(txCtx, poolID) // 自动走 tx
//	    if err != nil { return err }
//	    ...
//	    return nil
//	})
//
// 不传 txCtx 而传 c：repo 内部仍调 repository.DB(c)，会拿到原始 DB 而非 tx，
// 拆出 txCtx 是为了让 repo 在事务内复用同一条 DB 连接。
func RunInTx(c *gin.Context, fn func(txCtx *gin.Context) error) (err error) {
	db := DB(c)
	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("repository: tx begin: %w", tx.Error)
	}

	txCtx := c.Copy()
	txCtx.Set(database.CtxKey, tx)

	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback().Error
			panic(p)
		}
	}()

	if err = fn(txCtx); err != nil {
		if rbErr := tx.Rollback().Error; rbErr != nil {
			return fmt.Errorf("repository: tx rollback failed: %v (original: %w)", rbErr, err)
		}
		return err
	}
	if cmErr := tx.Commit().Error; cmErr != nil {
		return fmt.Errorf("repository: tx commit: %w", cmErr)
	}
	return nil
}

