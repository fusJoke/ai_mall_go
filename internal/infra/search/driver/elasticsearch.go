package driver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
)

// esDriver 是 search.SearchDriver 的 Elasticsearch 实现。
//
// 选型：github.com/elastic/go-elasticsearch/v8（官方维护的 ES Go 客户端，
// 与 ES 8.x 兼容，向下兼容 7.x）。
//
// 设计要点：
//   - 不在构造期 Ping，由 search.Init() 显式调 Ping 一次做 fail fast。
//   - BulkIndex 内部组装 NDJSON（每两行一对：metadata + source），
//     调 _bulk 接口单请求提交。
//   - Search 用 go-elasticsearch 的 esapi.Search 接口，query 字节流直接透传。
//   - 文档 / 索引命名按 index_prefix + 业务名拼接（如 mall_blind_box_index），
//     由 cmd/es-sync 在写入时负责；driver 自身不感知。
type esDriver struct {
	client *elasticsearch.Client
}

// newElasticsearchDriver 按 ElasticsearchConfig 构建 ES 客户端。
//
// addresses 必填（至少一个）；username/password 留空表示无鉴权。
func newElasticsearchDriver(cfg ElasticsearchConfig) (*esDriver, error) {
	if len(cfg.Addresses) == 0 {
		return nil, errors.New("search: elasticsearch addresses is empty")
	}

	esCfg := elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	}

	client, err := elasticsearch.NewClient(esCfg)
	if err != nil {
		return nil, fmt.Errorf("search: new elasticsearch client: %w", err)
	}

	return &esDriver{client: client}, nil
}

// Ping 健康检查；Init 阶段调用，失败 fail fast。
//
// 调用 _cluster/health 接口，期望 200 OK 即视为健康。
// 失败返回原始 error，由上层 log.Fatal。
func (d *esDriver) Ping(ctx context.Context) error {
	res, err := d.client.Cluster.Health(d.client.Cluster.Health.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("search: elasticsearch ping: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("search: elasticsearch ping status=%s body=%s", res.Status(), string(body))
	}
	return nil
}

// Index 单文档 upsert。
//
// PUT /{index}/_doc/{id}，body 是文档 JSON 字节流。
func (d *esDriver) Index(ctx context.Context, index, id string, body []byte) error {
	res, err := d.client.Index(
		index,
		bytes.NewReader(body),
		d.client.Index.WithContext(ctx),
		d.client.Index.WithDocumentID(id),
	)
	if err != nil {
		return fmt.Errorf("search: elasticsearch index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		respBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("search: elasticsearch index status=%s body=%s", res.Status(), string(respBody))
	}
	return nil
}

// BulkIndex 批量索引文档。
//
// 组装 NDJSON：每条文档两行 —— 第一行 {"index":{"_index":"...","_id":"..."}}
// 第二行是文档 body。整体一次性 POST 到 _bulk 接口。
//
// 为什么选 _bulk 而不是循环 Index：单次 _bulk 一次 HTTP 往返，
// 比 N 次 PUT 快 N 倍（ES 官方建议批量大小 5-15MB / 1000-5000 文档）。
func (d *esDriver) BulkIndex(ctx context.Context, index string, docs []BulkDoc) error {
	var buf bytes.Buffer
	for _, doc := range docs {
		// 第一行：metadata
		meta := fmt.Sprintf(`{"index":{"_index":%q,"_id":%q}}`+"\n", index, doc.ID)
		buf.WriteString(meta)
		// 第二行：文档 body
		buf.Write(doc.Body)
		buf.WriteByte('\n')
	}

	res, err := d.client.Bulk(
		bytes.NewReader(buf.Bytes()),
		d.client.Bulk.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("search: elasticsearch bulk: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		respBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("search: elasticsearch bulk status=%s body=%s", res.Status(), string(respBody))
	}

	// 即使 HTTP 200，ES _bulk 仍可能部分条目失败（item-level errors）；
	// 解码 resp 检查 errors=true。
	var bulkResp struct {
		Errors bool `json:"errors"`
	}
	if err := json.NewDecoder(res.Body).Decode(&bulkResp); err != nil {
		return fmt.Errorf("search: elasticsearch bulk decode: %w", err)
	}
	if bulkResp.Errors {
		return errors.New("search: elasticsearch bulk: partial failures (see ES logs)")
	}
	return nil
}

// Delete 按 ID 删文档；不存在 no-op（ES Delete 对不存在 ID 返回 404，
// 这里把 404 当 no-op）。
func (d *esDriver) Delete(ctx context.Context, index, id string) error {
	res, err := d.client.Delete(
		index,
		id,
		d.client.Delete.WithContext(ctx),
	)
	if err != nil {
		return fmt.Errorf("search: elasticsearch delete: %w", err)
	}
	defer res.Body.Close()

	// 404 → 视为不存在，no-op。
	if res.StatusCode == http.StatusNotFound {
		return nil
	}
	if res.IsError() {
		respBody, _ := io.ReadAll(res.Body)
		return fmt.Errorf("search: elasticsearch delete status=%s body=%s", res.Status(), string(respBody))
	}
	return nil
}

// Search 执行查询，返回命中列表 + 总命中数。
//
// query 是 ES DSL JSON 字节流（业务侧自行序列化）。
// 返回的 hits 是 []Hit，Source 是原始 JSON 字节，业务侧自行反序列化。
func (d *esDriver) Search(ctx context.Context, index string, query []byte) (hits []Hit, total int64, err error) {
	res, err := d.client.Search(
		d.client.Search.WithContext(ctx),
		d.client.Search.WithIndex(index),
		d.client.Search.WithBody(bytes.NewReader(query)),
		d.client.Search.WithTrackTotalHits(true),
	)
	if err != nil {
		return nil, 0, fmt.Errorf("search: elasticsearch search: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		respBody, _ := io.ReadAll(res.Body)
		return nil, 0, fmt.Errorf("search: elasticsearch search status=%s body=%s", res.Status(), string(respBody))
	}

	var sr struct {
		Hits struct {
			Total struct {
				Value int64 `json:"value"`
			} `json:"total"`
			Hits []struct {
				ID     string          `json:"_id"`
				Score  float64         `json:"_score"`
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(res.Body).Decode(&sr); err != nil {
		return nil, 0, fmt.Errorf("search: elasticsearch search decode: %w", err)
	}

	hits = make([]Hit, 0, len(sr.Hits.Hits))
	for _, h := range sr.Hits.Hits {
		hits = append(hits, Hit{
			ID:     h.ID,
			Source: []byte(h.Source),
			Score:  h.Score,
		})
	}
	return hits, sr.Hits.Total.Value, nil
}

// Close 释放底层 HTTP 客户端资源（连接池等）。
// 当前未在 search 层暴露 Close 路径；后续如需 runtime 释放再补。
func (d *esDriver) Close() error {
	// go-elasticsearch v8 的 client 暂无显式 Close；预留 hook。
	// 内部 transport 由 net/http 默认机制管理。
	return nil
}
