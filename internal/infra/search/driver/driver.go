// Package driver 收容 search 各种后端实现（elasticsearch / opensearch ...）。
//
// 每个驱动一个文件，文件名即驱动名（如 elasticsearch.go）。
//
// 重要：driver 包定义 Hit / BulkDoc 与 SearchDriver 接口（这些类型出现在接口
// 方法签名上），search 包 import driver 并以 type alias 方式 re-export 给业务侧。
// 这种"driver → types；search → driver"单向依赖打破了"types 定义在 search"的循环。
//
// 设计要点（与 internal/infra/cache/driver 对齐）：
//   - SearchDriver 是接口，对外只暴露接口；具体实现不导出结构体，只导出构造函数。
//   - Ping 由 driver 实现：Init 阶段确认后端可达，失败返回 error
//     （main 调 log.Fatal 退出）。
//   - 所有方法的 ctx 由调用方传入，driver 内部用 ctx 控制超时 / 取消。
package driver

import (
	"context"

	"ai-go-mall/internal/infra/config"
)

// Hit 是单条搜索命中文档。
//
// MVP 阶段 ES 文档结构固定（设计 D7 投影），业务侧拿到 *Hit 后按需反序列化
// 自己的结构；不在 driver 包内嵌具体业务类型，保持 driver 包的"通用搜索能力"
// 定位，避免被 mall 业务绑死。
type Hit struct {
	// ID 文档 ID（与 MySQL 主键对齐）。
	ID string

	// Source 文档原始 JSON 字节流；业务侧自行 json.Unmarshal。
	Source []byte

	// Score 命中分数（用于排序展示）。
	Score float64
}

// BulkDoc 是 BulkIndex 单条文档。
type BulkDoc struct {
	ID   string
	Body []byte
}

// SearchDriver 是 search 后端的统一接口。
//
// 当前 4 个方法覆盖 MVP 业务用例；后续如需新增（如 Update / Mapping 管理），
// 扩展接口即可，但保留现有方法签名以保证已落地的调用方不破坏。
type SearchDriver interface {
	// Index 单文档 upsert。
	Index(ctx context.Context, index, id string, body []byte) error

	// BulkIndex 批量索引文档。
	BulkIndex(ctx context.Context, index string, docs []BulkDoc) error

	// Delete 按 ID 删文档；不存在 no-op。
	Delete(ctx context.Context, index, id string) error

	// Search 执行查询。
	Search(ctx context.Context, index string, query []byte) (hits []Hit, total int64, err error)

	// Ping 健康检查；Init 阶段调用，失败 fail fast。
	Ping(ctx context.Context) error
}

// ElasticsearchConfig 是构造 ES 驱动所需的最小配置子集。
type ElasticsearchConfig = config.ElasticsearchConfig

// NewElasticsearch 是 ES 驱动的工厂函数。
// 由 search.NewDriver 调用；不直接由业务代码使用。
func NewElasticsearch(cfg ElasticsearchConfig) (SearchDriver, error) {
	return newElasticsearchDriver(cfg)
}
