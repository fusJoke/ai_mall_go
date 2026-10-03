// Package migrate 封装 github.com/golang-migrate/migrate/v4 的构造与调用，
// 把"基于 SQL 文件的版本化 schema 迁移"暴露成 cmd/serve 启动钩子与 cmd/migrate
// CLI 共用的 API surface。
//
// 设计要点：
//   - DSN 直接复用 config.Get().Database.Write，独立 sql.Open 一个 *sql.DB。
//     不复用 GORM 的 *sql.DB（dbresolver 与 migrate 的生命周期耦合）；
//   - 文件 source 用 iofs driver 加载 os.DirFS(migrationsDir)；开发态默认
//     `cmd/migrate/migrations`，生产环境由 cmd/serve 通过 ldflags 在编译期
//     把 main 的 migrationsDir 变量改成固定绝对路径，避免运行期环境变量切换；
//   - 版本号来自 migration 文件名前缀（NNNNNN_name.{up,down}.sql），由 iofs
//     driver 解析，本包不重复解析文件名；
//   - 上游 sentinel（migrate.ErrNoChange / ErrDirty / ErrLocked）直接透传，
//     调用方用 errors.Is 判断即可；本包另外定义 ErrInvalidState 用于
//     "schema_migrations 当前 version 大于本地最大版本号"这类上游不会主动报错
//     但运行期容易撞到的"本地丢文件"边界。
package migrate

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	gormmysql "github.com/go-sql-driver/mysql" // 导入即注册 mysql driver，且用于 cfg.FormatDSN()
	migratelib "github.com/golang-migrate/migrate/v4"
	mysqlmigrate "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"

	"ai-go-mall/internal/infra/config"
)

// ErrInvalidState 是一个 sentinel 错误，用于本包在 ErrNoChange / ErrDirty /
// ErrLocked 之外识别"schema_migrations.current_version > 本地最大版本号"
// 这种"本地丢文件"边界。当上游对此状态直接报错时本 sentinel 也会通过 wrap 命中。
//
// 调用方应该用 errors.Is(err, ErrInvalidState) 判断。
var ErrInvalidState = errors.New("migrate: invalid state")

// ErrInvalidStateDetail 携带可读的上下文（DB version 与本地最大 version 对比）。
// 通过 wrap(ErrInvalidState, ...) 抛出，errors.Is 命中 ErrInvalidState sentinel。
type ErrInvalidStateDetail struct {
	Reason string
}

func (e *ErrInvalidStateDetail) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidState.Error(), e.Reason)
}

// Is 让 errors.Is 命中包级 ErrInvalidState sentinel。
func (e *ErrInvalidStateDetail) Is(target error) bool {
	return target == ErrInvalidState
}

// ErrNoMigrationApplied 是一个 sentinel 错误，表示 schema_migrations 还没有
// 任何 row —— 即"还没跑过 migrate up"的全新库，或被人工 force 到 -1 / 0 之后的状态。
//
// 这与 ErrInvalidState 在语义上完全不同：前者是"还没开始"，后者是"DB 跑得比
// 本地快"。Up() 必须对前者继续运行；对后者必须 fatal。
var ErrNoMigrationApplied = errors.New("migrate: no migration applied yet")

// ErrNoMigrationDetail 是 ErrNoMigrationApplied 的携带体。其 Is 让 errors.Is
// 命中上面的 sentinel。
type ErrNoMigrationDetail struct{}

func (e *ErrNoMigrationDetail) Error() string { return ErrNoMigrationApplied.Error() }
func (e *ErrNoMigrationDetail) Is(target error) bool {
	return target == ErrNoMigrationApplied
}

// Migrator 持有 *migratelib.Migrate 与其背后的 *sql.DB、iofs 使用的源目录。
// cmd/serve 启动路径不需要 Close（进程退出即可），CLI 路径需要在 runUp/runDown
// 返回前 Close。
type Migrator struct {
	m     *migratelib.Migrate
	db    *sql.DB
	dir   string
	close func() error
}

// New 根据 cfg + migrationsDir 构造迁移器。
//
// migrationsDir 为绝对路径或相对路径（相对 cwd）。如目录不存在 / 没有任何 *.sql，
// 返回明确错误。cfg.Write.Host 为空时也返回明确错误，不允许落到连接失败这种二义错误。
func New(cfg config.DatabaseConfig, migrationsDir string) (*Migrator, error) {
	if cfg.Write.Host == "" {
		return nil, fmt.Errorf("migrate: config.Get().Database.Write.Host is empty")
	}

	if migrationsDir == "" {
		return nil, fmt.Errorf("migrate: migrations dir is empty")
	}
	if _, err := os.Stat(migrationsDir); err != nil {
		return nil, fmt.Errorf("migrate: migrations dir %q: %w", migrationsDir, err)
	}

	dsn := buildMySQLDSN(cfg.Write)
	migrationsTable := cfg.Prefix + "schema_migrations"

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, fmt.Errorf("migrate: open mysql: %w", err)
	}
	// 保守的连接池上限：migrate 自身不需要大并发，启动路径与 CLI 都是低频调用。
	db.SetMaxOpenConns(2)
	db.SetMaxIdleConns(2)
	db.SetConnMaxLifetime(5 * time.Minute)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: ping: %w", err)
	}

	driver, err := mysqlmigrate.WithInstance(db, &mysqlmigrate.Config{
		MigrationsTable: migrationsTable,
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: init mysql driver: %w", err)
	}

	src, err := iofs.New(os.DirFS(migrationsDir), ".")
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: init iofs source: %w", err)
	}

	m, err := migratelib.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: build migrator: %w", err)
	}

	return &Migrator{
		m:   m,
		db:  db,
		dir: migrationsDir,
		close: func() error {
			_, sErr := m.Close()
			dErr := db.Close()
			if sErr != nil {
				return sErr
			}
			return dErr
		},
	}, nil
}

// Close 释放 *migratelib.Migrate（清内部 source 缓存）与 *sql.DB。
// 仅 CLI 路径必须调用；cmd/serve 启动路径让进程退出兜底。
func (m *Migrator) Close() error {
	if m == nil || m.close == nil {
		return nil
	}
	return m.close()
}

// Up 应用所有未应用的 up migration。
//
// 决策表：
//
//	Version / SchemaCheck 返回         行为
//	-------------------------------------------------
//	nil                                 常规 up；非 ErrNoChange 一律透传
//	ErrNoMigrationApplied (fresh DB)    继续走上游 Up()，上游会把首个迁移写入 schema_migrations
//	ErrInvalidState (DB > local)        fatal：本地文件被丢回
//	其他 err (含 ErrDirty 等)           fatal：原样透传
//	v > 0 && dirty                      fatal：人工 force 后再 up
func (m *Migrator) Up() error {
	v, dirty, vErr := m.VersionWithSchemaCheck()
	if vErr != nil {
		if !errors.Is(vErr, ErrNoMigrationApplied) {
			return vErr
		}
		// ErrNoMigrationApplied：fresh DB，让上游从 version=1 开始跑。
	} else if dirty {
		return fmt.Errorf("migrate: schema_migrations is dirty at version %d", v)
	}
	if err := m.m.Up(); err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		return err
	}
	return nil
}

// Down 回滚一个版本（spec 约定的 default 行为）。
func (m *Migrator) Down() error {
	if err := m.m.Down(); err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		return err
	}
	return nil
}

// Steps 按步数正向（n>0）/ 反向（n<0）迁移；step=0 是 no-op。
func (m *Migrator) Steps(n int) error {
	err := m.m.Steps(n)
	if err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		return err
	}
	return nil
}

// Goto 跳转到指定版本 V —— 应用或回滚至 V。
func (m *Migrator) Goto(v uint) error {
	if err := m.m.Migrate(v); err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		return err
	}
	return nil
}

// Version 返回 (version, dirty, nil)。两类特殊状态走 sentinel：
//   - schema_migrations 完全没 row → 返回 ErrNoMigrationApplied（fresh DB）
//   - row 存在但 version=0（例如 `force 0` 之后）→ 同样视为 fresh DB：
//     真实迁移文件从 1 开始，(0, false) 这种 row 在 Up() 里会让上游走
//     versionExists(0) 失败（"no migration found for version 0"）。把它
//     翻译成 ErrNoMigrationApplied 让 Up() 透明继续。
func (m *Migrator) Version() (uint, bool, error) {
	v, dirty, err := m.m.Version()
	if err != nil {
		if errors.Is(err, migratelib.ErrNilVersion) {
			return 0, false, &ErrNoMigrationDetail{}
		}
		return 0, false, err
	}
	if v == 0 {
		return 0, false, &ErrNoMigrationDetail{}
	}
	return v, dirty, nil
}

// VersionWithSchemaCheck 在 Version() 之上多做一次"DB version > 本地最大
// version"检测，命中时返回 *ErrInvalidStateDetail，让 Up 在启动路径中主动
// 拒绝"本地丢了 migration 文件后强行 up"。
//
// 三类返回：
//   - (v, dirty, ErrNoMigrationApplied)  → 全新库，让 Up() 继续
//   - (v, dirty, ErrInvalidStateDetail)  → 本地丢文件，必须 fatal
//   - (v, dirty, nil)                    → 状态正常
//
// 本地未发现任何 .sql 时 maxLocalVersion=0，但 v=0（fresh DB）会被 Version()
// 先一步翻译成 ErrNoMigrationApplied —— 这里不会因 max=0 误报。
func (m *Migrator) VersionWithSchemaCheck() (uint, bool, error) {
	v, dirty, err := m.Version()
	if err != nil {
		// 含 ErrNoMigrationApplied：一并透传，由调用方按"是否 fresh"分支处理。
		return v, dirty, err
	}
	max, lerr := m.maxLocalVersion()
	if lerr != nil {
		// 本地枚举失败不阻塞：up 自身会因为 iofs driver 无法定位版本而报错。
		return v, dirty, nil
	}
	if max < int(v) {
		return v, dirty, &ErrInvalidStateDetail{Reason: fmt.Sprintf(
			"DB version %d is greater than the largest version found in %s (%d); "+
				"either pull back the missing migration files or run `migrate force %d` after manual cleanup",
			v, m.dir, max, max,
		)}
	}
	return v, dirty, nil
}

// Force 把 version 强制设为 v，dirty 强制清除。仅用于"已知 V-1 已成功但 V
// 标 dirty"的人工恢复。
//
// v == 0 时翻译成 v == -1（删除 schema_migrations 行）。原因：upstream 在
// schema_migrations 写出 (0, false) 行后，下一次 Up() 会走到 versionExists(0)，
// 报 "no migration found for version 0"。我们的 wrapper 让 Force(0) 与
// "还没跑过 migration" 的效果一致 —— 这更符合用户意图，也让后续 Up() 透明生效。
func (m *Migrator) Force(v int) error {
	if v == 0 {
		v = -1
	}
	return m.m.Force(v)
}

// maxLocalVersion 从 migrationsDir 中枚举 *.up.sql 前缀的最大版本号。
// 不被 export，仅内部使用。
func (m *Migrator) maxLocalVersion() (int, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return 0, err
	}
	max := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		v, ok := parseVersionPrefix(e.Name())
		if !ok {
			continue
		}
		if v > max {
			max = v
		}
	}
	return max, nil
}

// buildMySQLDSN 拼装 golang-migrate 兼容的 MySQL DSN。
//
// 走 go-sql-driver/mysql 的 Config + FormatDSN()，与 internal/infra/database
// 包所用方式一致；关键在于 FormatDSN 会自动转义 user/pass/dbname 中的特殊字符
// （@, :, /, ?, &），手工 fmt.Sprintf 拼字符串会注入风险（Codex review #4）。
//
// 与 database 包唯一的差异：额外带 multiStatements 让 .up.sql 文件里的多条 DDL
// 可一次性执行；loc=Local 与 ParseTime 与 database 保持一致。
func buildMySQLDSN(c config.DBInstanceConfig) string {
	cfg := gormmysql.Config{
		User:            c.Username,
		Passwd:          c.Password,
		Net:             "tcp",
		Addr:            fmt.Sprintf("%s:%d", c.Host, c.Port),
		DBName:          c.DBName,
		ParseTime:       true,
		Loc:             time.Local,
		MultiStatements: true,
		// 同 internal/infra/database：字面量构造不会带上 mysql.NewConfig() 的
		// 默认值，AllowNativePasswords 零值是 false，会让 native password 账号
		// 直接连不上（迁移与运行时用的是同一个业务账号，必须一致）。
		AllowNativePasswords: true,
		Params:               map[string]string{"charset": "utf8mb4"},
	}
	return cfg.FormatDSN()
}

// parseVersionPrefix 解析文件名 `<version>_<name>.up.sql` / `<version>_<name>.down.sql`
// 的前 6 位数字，返回 (version, true) 或 (0, false)。其它前缀（如 4 位、字母开头）
// 不识别，让 caller 决定如何处理。
func parseVersionPrefix(name string) (int, bool) {
	if len(name) < 8 {
		return 0, false
	}
	v := 0
	for i := range 6 {
		ch := name[i]
		if ch < '0' || ch > '9' {
			return 0, false
		}
		v = v*10 + int(ch-'0')
	}
	if name[6] != '_' {
		return 0, false
	}
	return v, true
}
