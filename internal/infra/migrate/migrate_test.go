package migrate

import (
	"errors"
	"testing"

	migratelib "github.com/golang-migrate/migrate/v4"

	"ai-go-mall/internal/infra/config"
)

// TestParseVersionPrefix 覆盖合法 / 非法文件名前缀的解析。
func TestParseVersionPrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
		ok   bool
	}{
		{"up 合法", "000001_baseline.up.sql", 1, true},
		{"down 合法", "000001_baseline.down.sql", 1, true},
		{"大版本号", "123456_some.up.sql", 123456, true},
		{"缺下划线", "000001baseline.up.sql", 0, false},
		{"非数字前 6 位", "abcdef_baseline.up.sql", 0, false},
		{"短前缀", "1_baseline.up.sql", 0, false},
		{"空名", "", 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parseVersionPrefix(c.in)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if got != c.want {
				t.Fatalf("got = %d, want %d", got, c.want)
			}
		})
	}
}

// TestBuildMySQLDSN 覆盖 DSN 拼装的字段顺序与编码。
func TestBuildMySQLDSN(t *testing.T) {
	c := config.DBInstanceConfig{
		Host:     "127.0.0.1",
		Port:     3306,
		Username: "root",
		Password: "secret",
		DBName:   "mall",
	}
	dsn := buildMySQLDSN(c)
	wantSubstrings := []string{
		"root:secret@tcp(127.0.0.1:3306)/mall",
		"parseTime=true",
		"multiStatements=true",
		"charset=utf8mb4",
		"loc=Local",
	}
	for _, s := range wantSubstrings {
		if !contains(dsn, s) {
			t.Errorf("dsn missing %q: %s", s, dsn)
		}
	}
}

// TestErrInvalidState_Is 覆盖 errors.Is 对 ErrInvalidState 的命中。
//
// 这是 spec "Requirement: 启动阻塞与错误传播" 与
// "Scenario: DB 与本地不一致时的错误" 的可测部分：DB 端的 dirty/version
// 检测需要真 MySQL，但 wrap 出来的 ErrInvalidState 是纯 Go 逻辑，应该独立测。
func TestErrInvalidState_Is(t *testing.T) {
	detail := &ErrInvalidStateDetail{Reason: "DB version 5 > local 3"}
	if !errors.Is(detail, ErrInvalidState) {
		t.Fatalf("errors.Is(detail, ErrInvalidState) should be true")
	}
	if !errors.Is(detail, ErrInvalidState) { // sentinel identity
		t.Fatalf("errors.Is(detail, ErrInvalidState) should be true (idempotent)")
	}
	// 反向命中：基础 sentinel 自身不应被视作 detail
	if errors.Is(ErrInvalidState, detail) {
		t.Fatalf("errors.Is(ErrInvalidState, detail) should be false")
	}
	// 其它 sentinel 不应误命中
	if errors.Is(detail, migratelib.ErrNoChange) {
		t.Fatalf("errors.Is(detail, migrate.ErrNoChange) should be false")
	}
	// ErrInvalidState 与 ErrNoMigrationApplied 必须互不命中 ——
	// 它们是 #2 修复里拆开的两个语义，不能再共享 sentinel。
	if errors.Is(detail, ErrNoMigrationApplied) {
		t.Fatalf("errors.Is(detail, ErrNoMigrationApplied) should be false")
	}
}

// TestErrNoMigrationApplied_Is 覆盖 errors.Is 对 ErrNoMigrationApplied 的命中。
//
// ErrNoMigrationApplied / ErrInvalidState 是 🟥#2 修复里拆开的两个 sentinel：
// 前者代表 "fresh DB（继续 up）"，后者代表 "DB 跑得比本地快（fatal）"。
// 它们的 Is 必须互不干扰，否则 Up() 的分支逻辑会失效。
func TestErrNoMigrationApplied_Is(t *testing.T) {
	detail := &ErrNoMigrationDetail{}
	if !errors.Is(detail, ErrNoMigrationApplied) {
		t.Fatalf("errors.Is(detail, ErrNoMigrationApplied) should be true")
	}
	// 反向不应命中
	if errors.Is(ErrNoMigrationApplied, detail) {
		t.Fatalf("errors.Is(ErrNoMigrationApplied, detail) should be false")
	}
	// 也不能命中 ErrInvalidState —— 同一语义不能被重复用于两个分支
	if errors.Is(detail, ErrInvalidState) {
		t.Fatalf("errors.Is(detail, ErrInvalidState) should be false")
	}
	// 也不能命中 ErrNoChange（上游 sentinel）
	if errors.Is(detail, migratelib.ErrNoChange) {
		t.Fatalf("errors.Is(detail, ErrNoChange) should be false")
	}
}

// TestErrNoChange_Is 覆盖 upstream sentinel 透传（wrap 也不应该改变 errors.Is）。
func TestErrNoChange_Is(t *testing.T) {
	wrapped := errors.Join(errors.New("context"), migratelib.ErrNoChange)
	if !errors.Is(wrapped, migratelib.ErrNoChange) {
		t.Fatalf("errors.Is should propagate ErrNoChange through wrap")
	}
}

// TestNew_EmptyHost 覆盖 spec "Scenario: 配置文件缺失 database 节点"：
// cfg.Write.Host 为空时必须返回明确错误，不允许落到连接失败的二义错误。
func TestNew_EmptyHost(t *testing.T) {
	_, err := New(config.DatabaseConfig{}, "cmd/migrate/migrations")
	if err == nil {
		t.Fatalf("New must return error when Host is empty")
	}
	if !contains(err.Error(), "Host is empty") {
		t.Fatalf("err should mention host emptiness: %v", err)
	}
}

// TestNew_InvalidDir 覆盖目录不存在时的明确错误。
func TestNew_InvalidDir(t *testing.T) {
	_, err := New(config.DatabaseConfig{Write: config.DBInstanceConfig{Host: "127.0.0.1"}}, "nonexistent_dir_xyz")
	if err == nil {
		t.Fatalf("New must return error when migrationsDir doesn't exist")
	}
	if !contains(err.Error(), "migrations dir") {
		t.Fatalf("err should mention migrations dir: %v", err)
	}
}

// contains 是 strings.Contains 的简化版，避免引入 strings 包（非必需）。
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
