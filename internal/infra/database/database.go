// Package database 负责基于配置创建并管理 GORM *gorm.DB 实例。
//
// 启动流程：调用 Init()，内部从 config.Get().Database 读取连接信息，
// 打开写库并按需注册读副本（dbresolver 插件自动按读写路由）。
//
// 调用方通过 Get() 拿到 *gorm.DB 后正常写 GORM 查询即可，
// 无需关心读写路由（事务内强制走写库）。
//
// 在 HTTP 请求作用域内，应通过 DBMiddleware + FromContext（或
// handler.DB / repository.DB 包装层）拿到绑定到 c.Request.Context()
// 的 *gorm.DB，以实现客户端取消传播与请求间隔离。
package database

import (
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	gormmysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
	"gorm.io/plugin/dbresolver"

	"ai-go-mall/internal/infra/config"
)

// CtxKey 是 gin.Context 中存储请求作用域 *gorm.DB 的 key。
// 取值时通过 FromContext（或 handler.DB / repository.DB）读取。
const CtxKey = "ai_go_mall.db"

var db *gorm.DB

// Init 读取 config.Get().Database，按 type 打开写库 GORM 实例，
// 在 Read.Enabled=true 时通过 dbresolver 插件注册读副本。
//
// schema 管理职责已迁移到 internal/infra/migrate（golang-migrate 文件源）；
// 本函数不再调用 GORM AutoMigrate。新增业务表必须写一份 cmd/migrate/migrations/
// 下的 SQL 文件，由启动钩子 migrate.Up() 统一建表。
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
func Init() error {
	if db != nil {
		return nil
	}

	dbCfg := config.Get().Database

	opened, err := openWrite(dbCfg)
	if err != nil {
		return err
	}

	if dbCfg.Read.Enabled {
		if err := registerReadReplica(opened, dbCfg); err != nil {
			return err
		}
	}

	db = opened
	return nil
}

// Get 返回已初始化的 *gorm.DB。Init 未调用过或失败时返回 nil。
func Get() *gorm.DB {
	return db
}

// Reset 清空已缓存的 *gorm.DB。主要用于测试。
func Reset() {
	db = nil
}

// DBMiddleware 是 Gin 中间件：为每个请求把全局 *gorm.DB 用 c.Request.Context()
// 绑定一次，再以 CtxKey 写入 gin.Context。
//
// 后续 handler / repository 通过 FromContext（或 handler.DB / repository.DB）
// 拿到绑定后的 *gorm.DB，从而：
//   - 客户端断开 → 请求 ctx 取消 → GORM 查询自动中止
//   - 多个并发请求各自的 session 互不干扰（共享底层连接池，但 session 信息隔离）
//
// 中间件应在 gin.Recovery 之后注册，以便把 "Init 未调用" 的 panic 转成 500。
// 全局 *gorm.DB 未初始化（Init 未调用）时直接 panic，由 Recovery 兜底。
func DBMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		base := Get()
		if base == nil {
			panic("database: *gorm.DB not initialized; did you call database.Init()?")
		}
		c.Set(CtxKey, base.WithContext(c.Request.Context()))
		c.Next()
	}
}

// FromContext 取出 DBMiddleware 注入的、绑定到当前请求 ctx 的 *gorm.DB。
//
// 缺失或类型错误时 panic —— 这是编程错误（中间件未注册或调用方越界），
// 应在测试期被发现，由上游 gin.Recovery 转成 500。
func FromContext(c *gin.Context) *gorm.DB {
	v, ok := c.Get(CtxKey)
	if !ok {
		panic("database: *gorm.DB not in gin.Context; did you register database.DBMiddleware()?")
	}
	db, ok := v.(*gorm.DB)
	if !ok {
		panic(fmt.Sprintf("database: unexpected type %T in gin.Context under key %q", v, CtxKey))
	}
	return db
}

// openWrite 按配置打开写库 GORM 实例，并配置其底层 *sql.DB 连接池。
func openWrite(dbCfg config.DatabaseConfig) (*gorm.DB, error) {
	dsn := buildMySQLDSN(dbCfg.Write)

	opened, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix: dbCfg.Prefix,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("open write database: %w", err)
	}

	if err := applyPool(opened, dbCfg.Write); err != nil {
		return nil, fmt.Errorf("configure write pool: %w", err)
	}
	return opened, nil
}

// registerReadReplica 通过 dbresolver 插件为已打开的 GORM 实例注册读副本。
//
// 插件会按操作类型自动路由：
//   - Create / Update / Delete / Exec / 事务 → 写库
//   - Find / First / Take / Raw（非事务） → 读副本
//
// 注意：dbresolver 在 Sources 留空时会把 opened 自身的连接池当作 source，
// SetMax* 回调会同时打到写库与读副本，把刚才在 openWrite 里设置的写库连接池
// 覆盖成读副本的数值。这里在 Use 之后再用 dbCfg.Write 把写库池重新刷一次。
func registerReadReplica(opened *gorm.DB, dbCfg config.DatabaseConfig) error {
	dsn := buildMySQLDSN(dbCfg.Read.DBInstanceConfig)

	resolver := dbresolver.Register(dbresolver.Config{
		// 不显式声明 Sources，让插件复用 gorm.Open 时打开的写库连接。
		Replicas: []gorm.Dialector{mysql.Open(dsn)},
	}).
		SetMaxOpenConns(dbCfg.Read.MaxOpenConns).
		SetMaxIdleConns(dbCfg.Read.MaxIdleConns).
		SetConnMaxLifetime(dbCfg.Read.ConnMaxLifetime).
		SetConnMaxIdleTime(dbCfg.Read.ConnMaxIdleTime)

	if err := opened.Use(resolver); err != nil {
		return fmt.Errorf("register dbresolver: %w", err)
	}

	// 写库连接池在 dbresolver 回调里被读副本配置覆盖了一次，按写库配置重新刷回。
	if err := applyPool(opened, dbCfg.Write); err != nil {
		return fmt.Errorf("reconfigure write pool after dbresolver: %w", err)
	}
	return nil
}

// applyPool 把连接池配置写到 GORM 实例底层的 *sql.DB。
func applyPool(opened *gorm.DB, c config.DBInstanceConfig) error {
	sqlDB, err := opened.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(c.MaxOpenConns)
	sqlDB.SetMaxIdleConns(c.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(c.ConnMaxLifetime)
	sqlDB.SetConnMaxIdleTime(c.ConnMaxIdleTime)
	return nil
}

// buildMySQLDSN 通过 go-sql-driver/mysql 的 Config 组装 DSN，
// 自动处理用户名 / 密码中的特殊字符转义。
func buildMySQLDSN(c config.DBInstanceConfig) string {
	cfg := gormmysql.Config{
		User:      c.Username,
		Passwd:    c.Password,
		Net:       "tcp",
		Addr:      fmt.Sprintf("%s:%d", c.Host, c.Port),
		DBName:    c.DBName,
		ParseTime: true,
		Loc:       time.Local,
		Params:    map[string]string{"charset": "utf8mb4"},
	}
	return cfg.FormatDSN()
}
