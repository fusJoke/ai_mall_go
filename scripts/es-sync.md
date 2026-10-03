# es-sync — MySQL → Elasticsearch 全量同步脚本

`cmd/es-sync` 把 mall 业务数据全量同步到 ES 索引，供 C 端首页 Feed 读取
（spec Requirement "ES sync script"；design D6 / D7）。

## 同步内容

| 索引 | 来源条件（预过滤） | 文档字段 |
|---|---|---|
| `mall_blind_box_index` | 盲盒 `status=active AND on_sale=true AND is_featured=true` | id / supplier_id / supplier_name / name / cover / price / promo_price（当前生效活动价，无活动则省略）/ rarity_summary（卡池权重百分比公示）/ hot_score（MVP 恒 0）/ created_at |
| `mall_supplier_index` | 供应商 `status=active AND is_featured=true` | id / name / logo / blind_box_count（在售盒数）/ featured_rank（MVP 恒 0） |

同步策略为**全量覆盖**：BulkIndex upsert 全部满足条件的文档，并删除
ES 中已不再满足条件的 stale 文档；脚本幂等，可重复运行。

## 运行方式

```bash
# 开发态：项目根目录下运行（读取 config/*.yaml 与 .env.yaml）
go run ./cmd/es-sync

# 编译后的二进制
go build -o bin/es-sync ./cmd/es-sync && ./bin/es-sync
```

前置条件：

- MySQL 可达（`config/database.yaml` + `.env.yaml`）；
- Elasticsearch 可达（`config/search.yaml`）——启动期 Ping，失败即非零退出；
- C 端首页 Feed 依赖本脚本产出，**首次部署或数据变更后需手动跑一次**。

## 触发方式（MVP）

| 方式 | 场景 | 说明 |
|---|---|---|
| 手动 | 开发态 / 演示前 | `go run ./cmd/es-sync` |
| CI pipeline | 部署阶段 | 部署 hook 在服务启动前执行一次，保证索引新鲜 |
| cron | 定时兜底（可选） | 例：`*/10 * * * * cd /path/to/ai-go-mall && go run ./cmd/es-sync >> var/log/es-sync.log 2>&1` |

为什么不做实时同步（CDC / binlog / 写时双写）：见 design D6 —— MVP 数据量小、
变更频率低，离线全量脚本足够且简单可调试。

## 输出示例

```
2026/10/02 12:00:00 mall_blind_box_index: 6 records synced in 43 ms
2026/10/02 12:00:00 mall_blind_box_index: 0 stale docs deleted
2026/10/02 12:00:00 mall_supplier_index: 2 records synced in 11 ms
2026/10/02 12:00:00 mall_supplier_index: 0 stale docs deleted
```

## 错误处理

- config / MySQL / ES 任一初始化失败 → `log.Fatalf`，退出码非零；
- 同步过程中任一查询 / 写入失败 → 立即中止并保留已写入部分（下次重跑自动补齐）；
- ES 完全不可用时，C 端首页接口返回 `500 + code=search_unavailable`（不 fallback MySQL）。
