package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeFile 是测试辅助：往 dir 写入 name + 内容（自动创建中间目录）。
func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// writeConfig 在 rootDir/config 下写入一个 yaml 文件。
func writeConfig(t *testing.T, rootDir, name, body string) {
	t.Helper()
	writeFile(t, filepath.Join(rootDir, "config"), name, body)
}

// writeEnv 在 rootDir 下写入 .env.yaml。
func writeEnv(t *testing.T, rootDir, body string) {
	t.Helper()
	writeFile(t, rootDir, EnvFile, body)
}

// freshRoot 创建一个临时根目录并重置 config 包全局状态。
func freshRoot(t *testing.T) string {
	t.Helper()
	Reset()
	return t.TempDir()
}

func TestInit_LoadsBaseConfig(t *testing.T) {
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: ai-go-mall
  port: 8080
database:
  type: mysql
  prefix: ""
  write:
    host: 127.0.0.1
    port: 3306
    username: root
    password: ""
    dbname: ai-go-mall
    max_open_conns: 100
    max_idle_conns: 10
    conn_max_lifetime: 1h
    conn_max_idle_time: 10m
  read:
    enabled: false
    host: 127.0.0.1
    port: 3306
    username: root
    password: ""
    dbname: ai-go-mall
    max_open_conns: 50
    max_idle_conns: 10
    conn_max_lifetime: 1h
    conn_max_idle_time: 10m
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got := Get()
	if got == nil {
		t.Fatal("Get() = nil after Init")
	}
	if got.Server.Name != "ai-go-mall" {
		t.Errorf("Server.Name = %q, want %q", got.Server.Name, "ai-go-mall")
	}
	if got.Server.Port != 8080 {
		t.Errorf("Server.Port = %d, want %d", got.Server.Port, 8080)
	}
	if got.Database.Type != "mysql" {
		t.Errorf("Database.Type = %q, want %q", got.Database.Type, "mysql")
	}
	if got.Database.Prefix != "" {
		t.Errorf("Database.Prefix = %q, want \"\"", got.Database.Prefix)
	}
	if got.Database.Write.MaxOpenConns != 100 {
		t.Errorf("Database.Write.MaxOpenConns = %d, want %d", got.Database.Write.MaxOpenConns, 100)
	}
	if got.Database.Write.ConnMaxLifetime != time.Hour {
		t.Errorf("Database.Write.ConnMaxLifetime = %v, want %v", got.Database.Write.ConnMaxLifetime, time.Hour)
	}
	if got.Database.Write.ConnMaxIdleTime != 10*time.Minute {
		t.Errorf("Database.Write.ConnMaxIdleTime = %v, want %v", got.Database.Write.ConnMaxIdleTime, 10*time.Minute)
	}
	if got.Database.Read.Enabled {
		t.Errorf("Database.Read.Enabled = true, want false")
	}
	if got.Database.Read.MaxOpenConns != 50 {
		t.Errorf("Database.Read.MaxOpenConns = %d, want %d", got.Database.Read.MaxOpenConns, 50)
	}
}

func TestInit_EnvOverridesBase(t *testing.T) {
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: prod-name
  port: 80
database:
  write:
    host: db.internal
    port: 3306
    username: prod_user
    password: prod_pass
    dbname: prod_db
`)
	writeEnv(t, root, `
server:
  name: dev-name
database:
  write:
    password: dev_pass
    dbname: dev_db
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got := Get()
	// 被 .env.yaml 覆盖的字段
	if got.Server.Name != "dev-name" {
		t.Errorf("Server.Name = %q, want %q (overridden by .env.yaml)", got.Server.Name, "dev-name")
	}
	if got.Database.Write.Password != "dev_pass" {
		t.Errorf("Database.Write.Password = %q, want %q (overridden by .env.yaml)", got.Database.Write.Password, "dev_pass")
	}
	if got.Database.Write.DBName != "dev_db" {
		t.Errorf("Database.Write.DBName = %q, want %q (overridden by .env.yaml)", got.Database.Write.DBName, "dev_db")
	}
	// 保留基础配置的字段
	if got.Server.Port != 80 {
		t.Errorf("Server.Port = %d, want %d (kept from base)", got.Server.Port, 80)
	}
	if got.Database.Write.Host != "db.internal" {
		t.Errorf("Database.Write.Host = %q, want %q (kept from base)", got.Database.Write.Host, "db.internal")
	}
	if got.Database.Write.Username != "prod_user" {
		t.Errorf("Database.Write.Username = %q, want %q (kept from base)", got.Database.Write.Username, "prod_user")
	}
}

func TestInit_WorksWithoutEnvFile(t *testing.T) {
	// 缺失 .env.yaml 不应报错，直接使用 config/*.yaml。
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: ai-go-mall
  port: 8080
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init without .env.yaml: %v", err)
	}
	if got := Get().Server.Name; got != "ai-go-mall" {
		t.Errorf("Server.Name = %q, want %q", got, "ai-go-mall")
	}
}

func TestInit_SkipsExampleTemplate(t *testing.T) {
	// config/ 内 *.example 结尾的模板文件应被跳过（不只限于 .env.yaml.example）。
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: real-config
database:
  write:
    host: real-host
`)
	writeConfig(t, root, "database.yaml.example", `
server:
  name: example-template
database:
  write:
    host: should-not-load
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got := Get()
	if got.Server.Name != "real-config" {
		t.Errorf("Server.Name = %q, want %q (example file must be skipped)", got.Server.Name, "real-config")
	}
	if got.Database.Write.Host != "real-host" {
		t.Errorf("Database.Write.Host = %q, want %q (example file must be skipped)", got.Database.Write.Host, "real-host")
	}
}

func TestInit_DefaultsDatabaseTypeToMySQL(t *testing.T) {
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: ai-go-mall
database:
  write:
    host: 127.0.0.1
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := Get().Database.Type; got != "mysql" {
		t.Errorf("Database.Type = %q, want %q (default)", got, "mysql")
	}
}

func TestInit_DefaultsReadReplicaToDisabled(t *testing.T) {
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: ai-go-mall
database:
  type: mysql
  write:
    host: 127.0.0.1
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := Get().Database.Read.Enabled; got {
		t.Errorf("Database.Read.Enabled = true, want false (default)")
	}
}

func TestInit_MergesMultipleBaseFiles(t *testing.T) {
	root := freshRoot(t)
	writeConfig(t, root, "server.yaml", `
server:
  name: from-server-yaml
`)
	writeConfig(t, root, "database.yaml", `
database:
  write:
    host: db.example.com
    port: 3306
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	got := Get()
	if got.Server.Name != "from-server-yaml" {
		t.Errorf("Server.Name = %q, want %q", got.Server.Name, "from-server-yaml")
	}
	if got.Database.Write.Host != "db.example.com" {
		t.Errorf("Database.Write.Host = %q, want %q", got.Database.Write.Host, "db.example.com")
	}
}

func TestInit_EnvOverridesAcrossMultipleFiles(t *testing.T) {
	// 验证 .env.yaml 在任意 config/*.yaml 之后合并，覆盖所有同名键。
	root := freshRoot(t)
	writeConfig(t, root, "server.yaml", `
server:
  name: from-server
`)
	writeConfig(t, root, "zzz_last.yaml", `
server:
  name: from-zzz
`)
	writeEnv(t, root, `
server:
  name: from-env
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if got := Get().Server.Name; got != "from-env" {
		t.Errorf("Server.Name = %q, want %q (env must always win)", got, "from-env")
	}
}

func TestInit_NoConfigFiles(t *testing.T) {
	root := freshRoot(t)
	if err := Init(root); err == nil {
		t.Fatal("Init = nil error, want error when no config file exists")
	}
}

func TestInit_Idempotent(t *testing.T) {
	root := freshRoot(t)
	writeConfig(t, root, "config.yaml", `
server:
  name: ai-go-mall
`)

	if err := Init(root); err != nil {
		t.Fatalf("Init (first): %v", err)
	}
	// 第二次调用应该是 no-op，且仍能拿到第一次的配置。
	if err := Init(root); err != nil {
		t.Fatalf("Init (second): %v", err)
	}
	if got := Get().Server.Name; got != "ai-go-mall" {
		t.Errorf("Server.Name after second Init = %q, want %q", got, "ai-go-mall")
	}
}
