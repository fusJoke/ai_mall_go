# Tasks

## 1. Model 定义

- [ ] 1.1 新建 `internal/model/config.go`，按 `design.md` D1-D8 节定义 `Config` 结构体（13 业务字段 + 3 时间戳字段 + `gorm.DeletedAt` 软删除），实现 `TableName(namer schema.Namer) string`（走 `namer.TableName("config")`），加 `init() { Register(Config{}) }` 自注册。验证：`gofmt -l internal/model/config.go` 输出为空；文件内 `grep` 出现 `Register(Config{})`。

- [ ] 1.2 `go build ./...`，确认编译通过（无 import 错误、无未使用变量）。验证：命令退出码 0。

## 2. 注册表测试

- [ ] 2.1 在 `internal/model/model_test.go` 的 `TestRegister_AndAll` 内追加 `Config` 断言：扩展 `got` 检查，包含 `reflect.TypeOf(Config{})` 必须为 true。验证：`go test -run TestRegister_AndAll ./internal/model/...` 通过。

## 3. 编译与回归

- [ ] 3.1 `go test ./internal/model/...`，确认 `TestRegister_AndAll` / `TestRegister_DedupByType` / `TestRegister_Variadic` / `TestRegister_NilIgnored` / `TestAll_ReturnsCopy` 全绿。验证：测试输出 PASS，无 FAIL。

- [ ] 3.2 `go vet ./...`，确认无静态检查告警。验证：命令退出码 0。

## 4. AutoMigrate 验证

- [ ] 4.1 启动 `cmd/serve`（本地数据库可用时）：观察启动日志，确认 AutoMigrate 阶段创建了 `config` 表，包含全部 13 + 3 字段以及 `name` 唯一索引。验证：手动 `SHOW CREATE TABLE config`（或 `DESC config`）输出列名 / 类型与 `design.md` D1 节一致；`SHOW INDEX FROM config` 出现 `name` UNIQUE 索引。