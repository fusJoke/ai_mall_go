// Package search 提供搜索的统一抽象与 Elasticsearch 实现。
//
// 设计（mall MVP 设计 D6 / D7）：
//   - SearchClient 接口：Index / BulkIndex / Delete / Search 四类操作。
//   - 应用进程只读 ES —— Index / BulkIndex / Delete 仅供 cmd/es-sync 调用，
//     业务侧只调 Search。
//   - 驱动放 driver/ 子目录，命名与 cache/driver/ 对齐。
//   - Init() 读 config.Get().Search.Elasticsearch，构造 ES 客户端 + Ping。
//
// 后续扩展（不在本期）：
//   - 真实业务 Search 调用（首页 feed 走 ES）在 Phase 3.5（service/user/home.go）落地。
//   - cmd/es-sync 全量同步脚本在 Phase 5 落地。
//
// 启动流程：cmd/serve/main.go 在 cache.Init() 之后调 search.Init()，
// 内部 Ping ES，失败 log.Fatal（spec fail fast）。
package search

import (
	"context"
	"fmt"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/search/driver"
)

// Sentinel errors：调用方通过 errors.Is 区分业务分支。
//
// 当前 driver 实现下，索引不存在 / 文档不存在由 driver 返回的 sentinel 表达。
var (
	// ErrSearchInvalid 入参非法（空 index / 空 query 等）。
	ErrSearchInvalid = fmt.Errorf("search: invalid input")
)

// Hit 是单条搜索命中文档（type alias，真实定义在 driver 包）。
//
// 业务侧拿到 *Hit 后按需反序列化自己的结构；不在 search 包内嵌具体业务类型，
// 保持 search 包的"通用搜索能力"定位，避免被 mall 业务绑死。
type Hit = driver.Hit

// BulkDoc 是 BulkIndex 单条文档（type alias）。
type BulkDoc = driver.BulkDoc

// SearchClient 是搜索的对外门面接口。
//
// 四类操作覆盖 cmd/es-sync + 业务侧的全部用例：
//   - Index：单文档 upsert（es-sync 也用，但通常走 BulkIndex）。
//   - BulkIndex：批量索引，es-sync 主力调用。
//   - Delete：按 ID 删文档（es-sync 全量覆盖前的"清表"步骤）。
//   - Search：执行查询，返回命中列表。
type SearchClient interface {
	// Index 单文档 upsert。
	Index(ctx context.Context, index string, id string, body []byte) error

	// BulkIndex 批量索引文档。
	// docs 是 (id, body) 对列表；driver 内部组装 NDJSON 调 _bulk。
	BulkIndex(ctx context.Context, index string, docs []BulkDoc) error

	// Delete 按 ID 删文档；不存在 no-op。
	Delete(ctx context.Context, index, id string) error

	// Search 执行查询，返回命中列表 + 总命中数。
	// query 是 ES DSL JSON 字节流（业务侧自行序列化）。
	Search(ctx context.Context, index string, query []byte) (hits []Hit, total int64, err error)
}

// Manager 是 search 业务的对外门面，持有 driver.SearchDriver。
//
// 与 cache.Manager 风格一致；Reset() 用于测试。
type Manager struct {
	driver driver.SearchDriver
}

// mgr 是 Init 缓存的 Manager 单例。Init 未调用或失败时为 nil。
var mgr *Manager

// Init 读取 config.Get().Search.Elasticsearch，按配置实例化 driver，
// 包装成 Manager 缓存到包级变量。
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
//
// Init 内部 Ping ES 一次，连接失败返回 error（spec fail fast）。
func Init() error {
	if mgr != nil {
		return nil
	}

	cfg := config.Get().Search.Elasticsearch
	d, err := newDriver(cfg)
	if err != nil {
		return err
	}

	// Ping 一次确认 ES 可达；失败 fail fast。
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := d.Ping(ctx); err != nil {
		return fmt.Errorf("search: ping driver: %w", err)
	}

	mgr = &Manager{driver: d}
	return nil
}

// Get 返回已初始化的 *Manager。Init 未调用过或失败时返回 nil。
func Get() *Manager {
	return mgr
}

// Reset 清空 Manager 缓存。专供测试使用。
func Reset() {
	mgr = nil
}

// --- Manager 转发方法（参数校验后转 driver） ---

// Index 单文档 upsert。
func (m *Manager) Index(ctx context.Context, index, id string, body []byte) error {
	if index == "" || id == "" {
		return ErrSearchInvalid
	}
	return m.driver.Index(ctx, index, id, body)
}

// BulkIndex 批量索引文档。
func (m *Manager) BulkIndex(ctx context.Context, index string, docs []BulkDoc) error {
	if index == "" || len(docs) == 0 {
		return ErrSearchInvalid
	}
	return m.driver.BulkIndex(ctx, index, docs)
}

// Delete 按 ID 删文档；不存在 no-op。
func (m *Manager) Delete(ctx context.Context, index, id string) error {
	if index == "" || id == "" {
		return ErrSearchInvalid
	}
	return m.driver.Delete(ctx, index, id)
}

// Search 执行查询。
func (m *Manager) Search(ctx context.Context, index string, query []byte) (hits []Hit, total int64, err error) {
	if index == "" {
		return nil, 0, ErrSearchInvalid
	}
	return m.driver.Search(ctx, index, query)
}

// newDriver 是 driver 工厂：根据配置实例化对应驱动。
// 当前仅 ES 一个驱动；后续如需 opensearch / solr 等加 case。
func newDriver(cfg config.ElasticsearchConfig) (driver.SearchDriver, error) {
	// 当前固定走 ES；后续如要按 driver 切换，加 cfg.Driver 字段。
	return driver.NewElasticsearch(cfg)
}
