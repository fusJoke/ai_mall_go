// Package repository 是数据访问层，只负责 DB ↔ 内存 的搬运，不掺业务逻辑。
//
// 当前文件仅放跨 repository 共用的小工具；具体 repository 后续按业务实体划分。
package repository

import (
	"fmt"
	"strconv"
	"sync/atomic"

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

// RunInTx 在事务内执行 fn，并支持嵌套。
//
// 行为：
//  1. 用 repository.DB(c) 拿到请求作用域 DB；
//  2. 未处于事务中 → 开新 tx（BEGIN/COMMIT/ROLLBACK）；
//     已处于事务中 → 用同名语义的 SAVEPOINT 实现嵌套（见下「嵌套」说明）；
//  3. 把事务句柄写进 c.Copy() 出来的 txCtx 的 CtxKey；
//  4. fn 在 txCtx 上跑，期间任何 repository.DB(txCtx) 都拿到同一个事务句柄；
//  5. fn 返回 nil → 提交；返回 error → 回滚，并把 fn 错误上抛；
//  6. fn 内部 panic → 回滚 + 重新 panic（保留调用栈）；
//  7. 提交/回滚本身出错 → wrap 到原 error 上返回（不静默吞）。
//
// 为什么用 c.Copy()：gin.Context.Set 会改内部 map，必须 copy 避免污染父 ctx；
// c.Copy() 是 gin 提供的官方安全拷贝。
//
// # 嵌套（OCR 审查 P1-5 发现）
//
// GORM 的 DB.Begin() **不支持嵌套** —— 已处于事务时其 Statement.ConnPool 是
// *sql.Tx，既不满足 TxBeginner 也不满足 ConnPoolBeginner，会直接返回
// ErrInvalidTransaction。本函数最初只走 Begin，因此「在事务里再调 RunInTx」
// 会立刻失败，让 service 层无法把多个自带事务的 repo 方法组合成一个原子单元。
//
// 现在按 GORM DB.Transaction 的同款判定（ConnPool 是否实现 TxCommitter）分流：
// 嵌套时改用 SAVEPOINT / ROLLBACK TO SAVEPOINT，语义与原生嵌套事务一致。
// SAVEPOINT 名由进程内自增计数器生成（MySQL 方言会把名字**内联**进 SQL，
// 不是绑定参数，因此不能含用户输入；自增数字天然安全且不会重名）。
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

	// 已在事务中 → 嵌套：用 SAVEPOINT 而非 BEGIN。
	//
	// 只看类型断言是否成立，不再额外判 `committer != nil`：
	// 那是个**永远为真**的判断 —— 接口持有 typed-nil 指针时 `!= nil` 仍为 true，
	// 而 `db.Begin()` 产出的 `*sql.Tx` 本就不可能是 typed-nil，
	// 留着只会让人误以为这里有 nil 保护。
	if _, ok := db.Statement.ConnPool.(gorm.TxCommitter); ok {
		return runInSavepoint(c, db, fn)
	}

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
			// 两个 %w：回滚失败与 fn 的原始错误都要能被 errors.Is/As 探到
			// （调用方常需要区分「业务错误」与「连接已断」两类根因）。
			return fmt.Errorf("repository: tx rollback failed: %w (original: %w)", rbErr, err)
		}
		return err
	}
	if cmErr := tx.Commit().Error; cmErr != nil {
		return fmt.Errorf("repository: tx commit: %w", cmErr)
	}
	return nil
}

// txSavepointSeq 为嵌套事务生成进程内唯一、纯数字的 SAVEPOINT 名。
// 纯数字是硬要求：MySQL 方言把名字内联进 `SAVEPOINT <name>`，不走绑定参数。
var txSavepointSeq atomic.Uint64

// runInSavepoint 在已有事务内用 SAVEPOINT 实现嵌套的事务边界。
//
// 与 GORM DB.Transaction 的嵌套分支保持同一语义：
//   - fn 出错 → ROLLBACK TO SAVEPOINT（只回滚嵌套块，外层事务仍可继续 / 提交）；
//   - fn 正常 → 不显式 RELEASE（与 GORM 一致：外层提交时统一释放），
//     因此回滚失败的报错必须上抛而不是静默吞。
func runInSavepoint(c *gin.Context, db *gorm.DB, fn func(txCtx *gin.Context) error) error {
	spName := "sp_rit_" + strconv.FormatUint(txSavepointSeq.Add(1), 10)
	if err := db.SavePoint(spName).Error; err != nil {
		return fmt.Errorf("repository: savepoint %s: %w", spName, err)
	}

	txCtx := c.Copy()
	txCtx.Set(database.CtxKey, db)

	defer func() {
		if p := recover(); p != nil {
			// 回滚失败在 panic 路径上**刻意**不上报：panic 必须以原值继续向上抛
			// （调用方可能做类型断言），而这里又没有可用的 logger —— repository 层
			// 不依赖 internal/infra/log，为了这一个分支破坏分层不划算。
			// 外层事务此时本已注定失败，丢失的只是一条次要诊断信息。
			_ = db.RollbackTo(spName).Error
			panic(p)
		}
	}()

	if err := fn(txCtx); err != nil {
		if rbErr := db.RollbackTo(spName).Error; rbErr != nil {
			// 同 RunInTx：回滚原因与原始错误都用 %w 包装，保留各自的 identity。
			return fmt.Errorf("repository: savepoint rollback failed: %w (original: %w)", rbErr, err)
		}
		return err
	}
	return nil
}

