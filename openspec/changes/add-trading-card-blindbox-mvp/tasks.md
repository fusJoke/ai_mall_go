# Tasks

## 1. 数据层与基础设施（对应 M1）

- [x] 1.1 新增迁移 `cmd/migrate/migrations/000006_mall_users.{up,down}.sql`：扩展 users 表，加 nickname/avatar/mobile/email/balance/status/last_login_at/last_login_ip/login_failure/password_hash 等字段，索引 `(username)`, `(mobile)`, `(email)`
- [x] 1.2 新增迁移 `000007_mall_suppliers.{up,down}.sql`：建 mall_suppliers + mall_supplier_users，索引 `(status, is_featured)`, `mall_supplier_users(supplier_id) UK`, `mall_supplier_users(username) UK`
- [x] 1.3 新增迁移 `000008_mall_blind_boxes.{up,down}.sql`：建 mall_cards + mall_blind_boxes + mall_card_pools + mall_card_pool_items，索引 `(supplier_id, status, on_sale)`, `(is_featured, status, on_sale)`, `mall_card_pools(blind_box_id) UK`, `mall_card_pool_items(pool_id, stock)`
- [x] 1.4 新增迁移 `000009_mall_draw_orders.{up,down}.sql`：建 mall_draw_orders + mall_draw_order_items，索引 `(user_id, created_at DESC)`, `(order_no) UK`, `mall_draw_order_items(order_id)`
- [x] 1.5 新增迁移 `000010_mall_promotions.{up,down}.sql`：建 mall_promotions，索引 `(blind_box_id, start_at) UK`, `(status, end_at)`
- [x] 1.6 新增迁移 `000011_mall_settlement.{up,down}.sql`：
  - 扩展 `mall_suppliers` 加 `balance` / `total_sales` / `commission_rate` 字段
  - 建 `mall_settlements`，索引 `(supplier_id, period_start, period_end) UK`, `(status, created_at)`
  - 建 `mall_settlement_items`，索引 `(settlement_id)`, `(draw_order_id) UK`
  - 建 `mall_platform_ledger`（counter_key 主键）
  - 初始化 `mall_platform_ledger` seed：`('total_revenue', 0)`、`('total_commission', 0)`
  - 初始化 `config.yaml` 加 `default_commission_rate: 0.1000`
- [x] 1.7 在 `internal/model/` 新建子包 `mall/`，按表拆分 model 文件（每个实体一文件，遵循 admin.go 风格：comment 优先 + 业务字段在前 + 时间戳收尾）
- [x] 1.8 扩展 `internal/model/user.go` 为 mall_users 结构（保持 Register 在 init 中）
- [x] 1.9 新增 `config/cache.yaml` + `config/search.yaml`，更新 `config/.env.yaml.example`
- [x] 1.10 新增 `internal/infra/cache/cache.go`：定义 `Cache` 接口（Get/Set/Del/SetNX），新增 `redis.go` 实现（用 `github.com/redis/go-redis/v9`），新增 `Init()` 全局初始化函数（fail fast）
- [x] 1.11 新增 `internal/infra/search/search.go`：定义 `SearchClient` 接口（Index/BulkIndex/Delete/Search），新增 `es.go` 实现（用 `github.com/elastic/go-elasticsearch/v8`），新增 `Init()` fail fast
- [x] 1.12 扩展 `cmd/serve/main.go`：在 `database.Init()` 之后调 `cache.Init()` + `search.Init()`
- [x] 1.13 扩展 `cmd/seed/`：补充种子数据（3 个供应商 + 6 个 supplier_users + 30 张 cards + 6 个盲盒 + 卡池 items + 2 个活跃活动）

## 2. Repository 层（所有身份）

- [x] 2.1 `internal/repository/user/`：新增 `user_repository.go`（CRUD + GetByUsername + DecrementBalance 条件更新）+ 单测
- [x] 2.2 `internal/repository/supplier/`：新增 `supplier_repository.go` + `supplier_user_repository.go` + 单测
- [x] 2.3 `internal/repository/mall/`：新增 `card_repository.go` + `blindbox_repository.go` + `cardpool_repository.go` + `promotion_repository.go` + `draw_order_repository.go`，每个 repository 配 sqlmock 单测
- [x] 2.4 `internal/repository/admin/`：扩展 `supplier_admin_repository.go`（list with filter + toggle_status + toggle_featured）+ `blindbox_admin_repository.go`（同上）

## 3. Service 层

- [x] 3.1 `internal/service/user/auth.go`：Login（密码校验 + 签发 token type="user"）+ 单测
- [x] 3.2 `internal/service/user/blindbox.go`：List（status=active AND on_sale，过滤供应商 disabled）+ Detail（带卡池 items + 当前活动 + 缓存读写）+ 单测
- [x] 3.3 `internal/service/user/draw.go`：核心事务逻辑（按 spec Requirement "Draw transaction atomicity" 实现 D3 算法）+ 单测 + 集成测试（用真实 MySQL 验证并发安全）
- [x] 3.4 `internal/service/user/order.go`：List（分页，按 created_at DESC）+ Detail + 单测
- [x] 3.5 `internal/service/user/home.go`：Feed（走 ES）+ 单测
- [x] 3.6 `internal/service/supplier/auth.go`：Login（同 user，token type="supplier"）+ 单测
- [x] 3.7 `internal/service/supplier/product.go`：List（自己的盲盒）+ Create（事务内同时建 pool）+ Update（含缓存失效）+ ToggleOnSale + 单测
- [x] 3.8 `internal/service/supplier/promotion.go`：List + Create（含时间重叠校验）+ Update + Toggle + Delete + 单测
- [x] 3.9 `internal/service/admin/supplier.go`：List（含 disabled）+ ToggleStatus + ToggleFeatured + 单测
- [x] 3.10 `internal/service/admin/blindbox.go`：List + ToggleStatus + ToggleOnSale + ToggleFeatured + 单测

## 4. Handler 与 Router 层

- [x] 4.1 `internal/middleware/rate_limit.go`：实现 `RateLimitPerMinute(uidExtractor func, n int)` gin middleware（Redis SetNX 算法），单测
- [x] 4.2 `internal/middleware/auth.go`：扩展现有 AdminAuth 模式，新增 `UserAuth` + `SupplierAuth`，按 token type 区分
- [x] 4.3 `internal/handler/user/auth.go`：Login HTTP 处理（错误码映射）
- [x] 4.4 `internal/handler/user/blindbox.go`：List + Detail
- [x] 4.5 `internal/handler/user/draw.go`：Draw（套 RateLimitPerMinute + UserAuth）
- [x] 4.6 `internal/handler/user/order.go`：List + Detail
- [x] 4.7 `internal/handler/user/home.go`：Feed
- [x] 4.8 `internal/handler/supplier/auth.go` + `product.go` + `promotion.go`：对应 service 暴露 HTTP
- [x] 4.9 `internal/handler/admin/supplier.go` + `blindbox.go`：admin 端管理
- [x] 4.10 `internal/router/user/router.go`：注册 7 个路由（含中间件）
- [x] 4.11 `internal/router/supplier/router.go`：注册 9 个路由
- [x] 4.12 `internal/router/admin/router.go`：扩展注册 supplier/blindbox/promotion/settlement 管理路由
- [x] 4.13 新增迁移 `000012_admin_rule_mall_menu（原计划 000014，编号顺延裁定见 ledger）.{up,down}.sql`：在 admin_rule 表插入 supplier / blindbox / promotion / seckill / settlement / follow 六组管理菜单的种子规则（编号顺延：000012/000013 预留给 Phase 9/18，裁定见 ledger）

## 5. cmd/es-sync 全量同步脚本

- [x] 5.1 新增 `cmd/es-sync/main.go`：读取 `internal/infra/database.Get()` + `internal/infra/search`，全量读 MySQL → 转投影 → 调 `BulkIndex` → 输出统计
- [x] 5.2 错误处理：DB 失败 / ES 失败 → log + 退出码非零
- [x] 5.3 文档：`scripts/es-sync.md`（README 一段：触发方式 = 手动 / CI / cron）

## 6. 前端

- [x] 6.1 扩展前端依赖检查：vue / vue-router / pinia / element-plus 已就位
- [x] 6.2 新增 `web/src/api/user/` + `web/src/api/supplier/`，每个端按 entity 拆文件
- [x] 6.3 新增 `web/src/lang/{zh-cn,en}/user.yaml` + `supplier.yaml`，i18n key 全覆盖（含错误码文案）
- [x] 6.4 新增 `web/src/layouts/user/` + `web/src/layouts/supplier/`：参考 admin layout 结构，主题色改篮球橙蓝
- [x] 6.5 新增 `web/src/router/static/userBase.ts` + `supplierBase.ts`，在主路由注册 `/user` 和 `/supplier` 顶层路由
- [x] 6.6 新增 `web/src/views/user/login.vue`
- [x] 6.7 新增 `web/src/views/user/home/index.vue`：ES feed 混合流（盲盒 + 供应商卡片）
- [x] 6.8 新增 `web/src/views/user/blindbox/detail.vue`：封面 + 概率公示表 + 限时特价信息 + 抽卡按钮 + 卡池预览
- [x] 6.9 新增 `web/src/views/user/orders/index.vue` + `[id].vue`
- [x] 6.10 新增 `web/src/views/supplier/login.vue` + `products/index.vue` + `products/edit.vue`
- [x] 6.11 扩展 `web/src/views/admin/`：新增 supplier / blindbox / promotion 管理页

## 7. 测试与文档

- [x] 7.1 全量 `go build ./...` 通过
- [x] 7.2 全量 `go test ./...` 通过（包含 sqlmock 单测 + service 单测 + handler 单测 + 集成测试）
- [x] 7.3 前端 `pnpm build` 通过
- [x] 7.4 手动验证主流程：登录 → 首页 → 盲盒详情 → 抽卡 → 看订单（用真实 MySQL + Redis + ES 跑通）
- [x] 7.5 手动验证供应商流程：登录 → 创建盲盒 + 卡池 → 创建活动 → 看后台是否同步
- [x] 7.6 手动验证 admin 流程：禁用供应商 → ES 同步脚本跑完后前端列表过滤
- [x] 7.7 更新 `CLAUDE.md` 项目状态：把 mall 业务加进去

## 8. 结算流程（对应 D13 / D14）

- [x] 8.1 扩展 `mall_suppliers` model：加 `Balance` / `TotalSales` / `CommissionRate *float64` 字段（指针表达可空）
- [x] 8.2 新增 `mall/settlement.go` model：`MallSettlement`（按 spec Requirement 字段）+ `MallSettlementItem` + `MallPlatformLedger`
- [x] 8.3 新增 `internal/repository/mall/settlement_repository.go`：CRUD + 业务专属方法（Generate 事务 / MarkPaid 事务 / Preview 不写库 / ListBySupplierWithPeriod）+ sqlmock 单测
- [x] 8.4 新增 `internal/service/admin/settlement.go`：
  - `Service` 接口：`List` / `Detail` / `Preview` / `Generate` / `MarkPaid`
  - sentinel errors：`ErrNoOrdersToSettle` / `ErrSettlementConflict` / `ErrSettlementAlreadyPaid` / `ErrInsufficientSupplierBalance`
  - 单测覆盖每种 sentinel error + 正常路径
- [x] 8.5 修改 `internal/service/user/draw.go`：在抽卡事务步骤 7 之后追加 7.5（UPDATE supplier balance + total_sales）+ 7.6（INSERT platform ledger counter），保证同事务原子
- [x] 8.6 新增 `internal/handler/admin/settlement.go`：5 个端点（List / Detail / Preview / Generate / MarkPaid）+ 错误码映射
- [x] 8.7 `internal/router/admin/router.go`：注册 5 条 settlement 路由，挂在 AdminAuth 中间件
- [x] 8.8 新增 `cmd/seed/` 扩展：种 1 个供应商 `commission_rate=0.0800`（VIP 折扣）+ 1 个供应商 NULL（走默认 10%）
- [x] 8.9 新增 `web/src/api/admin/settlement.ts`：5 个 API 函数
- [x] 8.10 新增 `web/src/views/admin/settlement/index.vue`：结算单列表（按状态/周期/供应商筛选）+ 行展开看 items
- [x] 8.11 新增 `web/src/views/admin/settlement/preview.vue` + `detail.vue`：预览 + 详情
- [x] 8.12 扩展 `web/src/lang/{zh-cn,en}/admin.yaml`：加 settlement.* i18n key
- [x] 8.13 集成测试：跑通"10 单抽卡 → admin 生成 09 月结算单 → preview → generate → 校验数字 → mark_paid → 校验 supplier.balance + platform_ledger 三方一致"（`internal/service/admin/settlement_integration_test.go` + `//go:build integration` tag，环境变量 `MYSQL_TEST_DSN` 提供 DSN 时跑通）
- [x] 8.14 集成测试：mark_paid 二次调用 → 409（同文件 `TestSettlementIntegration_MarkPaidIdempotent`）

## 9. 秒杀活动（对应 D15）

- [x] 9.1 新增迁移 `000013_mall_seckill.{up,down}.sql`（原计划 000012，编号顺延裁定见 ledger）：
  - 建 `mall_seckill_activities`（含 redis_initialized 字段）+ 索引 `(status, start_at, end_at)`
  - 建 `mall_stock_deduction_log`（含 synced_to_pool_at）+ 索引 `(synced_to_pool_at, deducted_at)` + `(seckill_id)`
- [x] 9.2 新增 model：`MallSeckillActivity` + `MallStockDeductionLog`
- [x] 9.3 新增 `internal/repository/mall/seckill_repository.go`：CRUD + 业务方法（CreateWithRedisInit 事务 / ListActive / IncrementStock / DecrementStock 原子）+ sqlmock 单测
- [x] 9.4 新增 `internal/service/supplier/seckill.go`：Create（含 Redis 库存初始化 + 卡池库存校验）+ Update + Toggle + List + 单测
- [x] 9.5 修改 `internal/service/user/draw.go`：新增 `DrawSeckill` 方法，按 D15 流程实现：Redis 限购 → Redis DECR → MySQL 事务（含 D3 抽卡 + 写 stock_deduction_log + 结算副作用）→ 失败回滚 + 异步 publish MQ
- [x] 9.6 新增 `internal/handler/user/seckill.go`：POST /api/v1/user/seckill/:id/draw，套 RateLimit + UserAuth
- [x] 9.7 新增 `internal/router/user/router.go` 注册秒杀路由
- [x] 9.8 新增 admin 接口：`POST /admin/seckill` `GET /admin/seckill` `POST /admin/seckill/:id/toggle`（admin 可禁用秒杀活动）
- [x] 9.9 cmd/seed 扩展：种 1 个 active 秒杀活动（NBA 全明星盲盒，秒杀 79.00，total_stock=100，per_user_limit=1，起 +1h 止 +7d），RedisInitialized=true（seed 不接 Redis，由 README 引导手动 SETNX 注入），幂等键 `(blind_box_id, start_at)`

## 10. 两层缓存架构（对应 D5 / D16）

- [x] 10.1 `internal/infra/cache/cache.go`：定义 `Cache` 接口（Get / Set / Del / SetNX / Incr / Decr）
- [x] 10.2 `internal/infra/cache/driver/memory.go`：L1 实现，用 `github.com/patrickmn/go-cache`（defaultExpiration + CleanupInterval）；MaxSize 字段保留未启用（v2.1.0 无内置 LRU）
- [x] 10.3 L2 Redis 实现：driver/redis.go 已存在，由 Manager 包成 Cache 接口（driver="redis" 路径）
- [x] 10.4 `internal/infra/cache/multi.go`：MultiLevelCache 组合（Get 按 L1→L2→DB 回填；Set 写 L2+L1；Del 失效 L2+L1；SetNX/Incr/Decr 仅 L2）
- [x] 10.5 `config/cache.yaml`：加 memory 配置（default_ttl=10s, cleanup_interval=60s, max_size=10000）+ ttl 段（blindbox_detail / home_feed / draw_rate 各自 L1/L2 TTL）
- [x] 10.6 `cache.Init()` 装配 MultiLevelCache（fail fast）：由 config.Cache.Driver 决定 "redis"（单层 Manager）或 "multi"（双层）
- [x] 10.7 `internal/service/user/blindbox.go` Detail：注入 CacheTTL（来自 config.Cache.TTL.BlindBoxDetail.L2），调 cache.Get / cache.Set（缓存即优化，回填失败不阻断）
- [x] 10.8 `internal/service/user/home.go` Feed：注入 Cache + CacheTTL（来自 config.Cache.TTL.HomeFeed.L2），缓存命中直返，miss 走 ES 并回填；CacheTTL=0 时跳过回填
- [x] 10.9 单测：memory driver（get/set/del/ttl/no-expire/默认 ttl/SetNX-Incr-Decr 返 ErrL1NotSupported）+ MultiLevelCache（L1 hit / L1 miss→L2 hit→回填 / 双 miss / Set 写双层 / Del 删双层 / SetNX/Incr/Decr 仅 L2）+ home Feed（cache hit 跳过 ES / miss 回填 / nil cache 兜底 / TTL=0 不回填）

## 11. MQ 多驱动抽象（对应 D17）

- [x] 11.1 新增 `internal/infra/mq/mq.go`：定义 `MQ` 接口（Publish / Subscribe）
- [x] 11.2 新增 `internal/infra/mq/redis.go`：Redis Stream 实现（XADD / XREADGROUP / XACK）
- [x] 11.3 新增 `internal/infra/mq/mock.go`：内存实现（单测用，map + chan）
- [x] 11.4 `config/mq.yaml`：mq.driver=redis, consumer_group=mall-stock-sync, block_timeout=5s
- [x] 11.5 `cmd/serve/main.go`：`mq.Init()` 装配 Redis Stream client（fail fast）
- [x] 11.6 新增 `cmd/stock-sync/main.go`：双模式（消费者 + cron 兜底），订阅 `stock.deduction.sync`，handler UPDATE `synced_to_pool_at`
- [ ] 11.7 修改 `internal/service/user/draw.go` 的 SeckillDraw：事务提交后 `go mq.Publish(...)`（失败仅 warn）
- [x] 11.8 单测：MQ Publish / Subscribe + Retry 行为
- [x] 11.9 修 `.env.yaml.example` 漏 mq 段：`config/mq.yaml` 的 redis.password 留空（按设计意图由
       `.env.yaml` 覆盖），但模板没列出 mq 段，导致复制 `.example` 后启动遇
       `init mq: NOAUTH` 失败。commit `b161fb6` 补回 mq 段，并在 design.md Cross-cutting
       Concerns 加上"敏感字段约定"+ 踩坑记录。

## 12. 秒杀活动前端

- [x] 12.1 `web/src/api/user/seckill.ts`：秒杀活动列表 + 秒杀下单接口
- [x] 12.2 `web/src/views/user/seckill/index.vue`：当前生效的秒杀活动列表（限时倒计时）
- [x] 12.3 `web/src/views/user/seckill/detail.vue`：秒杀活动详情（限购规则 + 实时剩余名额 + 秒杀按钮）
- [x] 12.4 `web/src/api/supplier/seckill.ts`：供应商秒杀管理接口
- [x] 12.5 `web/src/views/supplier/seckill/index.vue` + `edit.vue`：创建 / 编辑秒杀活动（含 total_stock / per_user_limit / 时间窗 / 卡池校验提示）
- [x] 12.6 `web/src/lang/{zh-cn,en}/{user,supplier}.yaml`：加 seckill.* i18n key
- [x] 12.7 前端集成：抽卡订单详情页要区分 source='seckill' 还是 'normal'，UI 提示用户

> 完成备注：页面所需的后端只读端点本节补齐（`internal/service/user/seckill_browse.go` List/Detail +
> handler + 路由；供应商秒杀 HTTP 端点 handler/supplier/seckill.go + 4 条路由，service 沿用 9.4）。
> 剩余名额口径：Redis `seckill:stock:{id}` 优先，miss 走 `CountDeductionLogsBySeckillIDs` DB 兜底，
> 双失败返回 -1。另修复两处阻塞前端渲染的历史缺陷：user/supplier.yaml 顶层 key 与文件路径双重嵌套
> （i18n 全部原样输出）、element-plus 组件从未全局注册（所有 el-* 页面运行时崩溃）。

## 13. 第三方渠道适配器 + 工厂（对应 D18）

- [x] 13.1 新增 `internal/infra/channel/channel.go`：定义 `Channel` 接口（Pay / Payout / Notify）+ `PayRequest` / `PayResult` / `PayoutRequest` / `PayoutResult` 等 DTO
- [x] 13.2 新增 `internal/infra/channel/factory.go`：实现 `ChannelFactory.Create(name string) (Channel, error)` + drivers 注册表
- [x] 13.3 新增 `internal/infra/channel/mock.go`：Mock 实现（MVP 默认，记录调用日志，返回成功）
- [x] 13.4 新增 `internal/infra/channel/bank.go`：银行实现占位（MVP = 调 mock，标 TODO）
- [x] 13.5 新增 `config/channel.yaml`：channel.payment=mock / channel.bank=mock / channel.sms=mock
- [x] 13.6 `cmd/serve/main.go`：`channel.Init()` 装配 ChannelFactory（fail fast）
- [x] 13.7 单测：Factory.Create 正确返回 driver；未知 name 返回 ErrUnknownChannel
- [x] 13.8 业务集成（解耦）：抽卡 service 与结算 service 通过 factory 调 channel（即使 MVP 是 mock，也走 channel 接口，方便未来替换）

> 完成备注：13.8 集成点 —— Draw / DrawSeckill 事务 COMMIT 后经 `factory.Create("payment").Pay`
> 记录模拟支付（`internal/service/user/channel_hooks.go`）；结算 MarkPaid 事务成功后经
> `factory.Create("bank").Payout` 记录打款（`baseSettlementService.recordPayout`）。
> 两处均事务外、失败仅 warn 不回滚业务（渠道记录是旁路审计）；factory 为 nil 时跳过。
> Init 校验 payment/bank/sms 三个配置名均可实例化（fail fast），mock/bank 驱动可创建性有单测，
> 并以临时端到端测试验证真实 config/*.yaml 加载后 Init 成功。

## 14. 订单状态机 - 状态模式（对应 D19）

- [x] 14.1 新增 `internal/domain/state/state.go`：定义 `State` 接口（Name / CanTransitionTo / OnEnter）+ `OrderContext` 接口 + `Transition` helper
- [x] 14.2 新增 `internal/domain/state/draw_order_state.go`：实现 `PendingState` / `PaidState` / `DrawnState` / `FailedState` + 合法转换表
- [x] 14.3 新增 `internal/domain/state/settlement_state.go`：实现 `PendingState` / `ProcessingState` / `PaidState` / `FailedState` + 合法转换表
- [x] 14.4 新增 `internal/domain/state/state_test.go`：单测覆盖每种合法转换 + 每种非法转换被拒绝 + 终态不可转换
- [x] 14.5 修改 `internal/service/user/draw.go`：在事务步骤 7（commit 前）调 `order.Transition(PaidState)`；步骤 9 后调 `order.Transition(DrawnState)`
- [x] 14.6 修改 `internal/service/admin/settlement.go`：Generate 后调 `settlement.Transition(ProcessingState)`；MarkPaid 后调 `settlement.Transition(PaidState)`
- [x] 14.7 集成测试：非法转换（如 pending → drawn）→ ErrInvalidStateTransition → HTTP 409

## 15. 下单校验责任链（对应 D20）

- [x] 15.1 新增 `internal/domain/chain/validator.go`：定义 `Validator` 接口（Name / Validate）+ `DrawInput` 输入结构
- [x] 15.2 新增 `internal/domain/chain/chain.go`：实现 `Chain` 结构 + `Validate(ctx, input) error`（顺序执行，任一 err 终止）
- [x] 15.3 新增 `internal/service/user/draw_check/user_active_check.go`：检查 mall_users.status=1
- [x] 15.4 新增 `blindbox_buyable_check.go`：检查 blind_box.status + on_sale + supplier.status
- [x] 15.5 新增 `time_window_check.go`：检查活动 / 秒杀 时间窗（传入 source 区分）
- [x] 15.6 新增 `user_limit_check.go`（秒杀）：Redis INCR seckill:user_bought
- [x] 15.7 新增 `balance_check.go`：检查 user.balance >= actual_price（LEFT JOIN 拿活动价）
- [x] 15.8 新增 `internal/service/user/draw_check/chain_builder.go`：组装 DrawChain / SeckillDrawChain
- [x] 15.9 修改 `internal/service/user/draw.go`：事务前调 `chain.Validate(ctx, input)`，校验失败 fast return
- [x] 15.10 单测：每个 Check 独立测；Chain 顺序执行；任一失败终止

> 完成备注：sentinel errors 集中在 `internal/domain/chain/errors.go`；
> `internal/service/user/draw.go` 用 `var ErrXxx = chain.ErrXxx` 别名以保持 handler 向后兼容；
> user_limit（Redis 副作用）作为 free function `AcquireUserLimit` 留在链外，由
> `DrawSeckill` 显式调用；seckill source 在 chain 前先预取 seckill→blind_box_id；
> `chain.ValidationError` 实现 `Unwrap` 让 `errors.Is(err, chain.ErrUserNotAvailable)`
> 等链路可透明穿透。

## 16. 热点数据缓存（对应 D21）

- [x] 16.1 新增 `internal/infra/cache/hotspot/hotspot.go`：定义 `HotspotCache` 接口（Get / Invalidate）+ `HotspotLoader` 函数类型
- [x] 16.2 新增 `internal/infra/cache/hotspot/redis_hotspot.go`：Redis 实现（GET / SET / 分布式锁）+ singleflight 合并 + Lua 脚本释放锁
- [x] 16.3 新增 `internal/infra/cache/hotspot/hotspot_test.go`：单测覆盖冷启动 / 逻辑时间触发异步刷新 / 并发竞争 / 主动失效 / Lua 锁释放
- [x] 16.4 `cmd/serve/main.go`：`hotspot.Init()` 装配 Redis HotspotCache（复用 Redis 客户端）
- [x] 16.5 修改 `internal/service/user/blindbox.go` 的 Detail：用 HotspotCache.Get 替换 TTL 缓存（staleAfter=30s）
- [x] 16.6 修改 `internal/service/user/home.go` 的 Feed：用 HotspotCache.Get 替换 TTL 缓存（staleAfter=60s）
- [x] 16.7 修改 `internal/service/supplier/product.go` 的 Update：事务提交后调 `hotspot.Invalidate("hotspot:blindbox:{id}")`
- [x] 16.8 依赖新增：`golang.org/x/sync`（已有，singleflight 子包）
- [x] 16.9 集成测试：100 并发请求 cold start → singleflight 合并后只 1 个打 DB；逻辑时间到期后 1 个抢锁异步刷新，其他 99 返回老数据

> 完成备注：hotspot 对 Redis 的能力面收敛为私有 `redisPort`（Get/Set/Del/SetNX/Eval），
> 生产适配 `cache.RawRedisClient()` 暴露的共享 go-redis 客户端（cache 新增 raw.go；
> Manager / MultiLevelCache 各实现 RawClient 断言转发）；单测用 fake port（逻辑路径）
> + miniredis（真实 Lua 语义）双轨。Detail / Feed 保留 Hotspot 未装配时的 TTL 回退路径，
> 旧单测零改动通过。异步刷新用 `context.WithoutCancel` + `database.DetachedGinContext`
> （新 helper）脱离请求生命周期跑 repository。写路径按 D21 约定双层失效
> （InvalidateDetail 签名扩为 cache + hotspot 两个句柄，product/promotion 共 5 处调用点）。
> 新增依赖：golang.org/x/sync（singleflight）、github.com/alicebob/miniredis/v2（test）。

## 17. 缓存防穿透（对应 D5.1 / D21 扩展）

- [x] 17.1 修改 `internal/infra/cache/cache.go`：`Cache` 接口的 `Get` 返回值增加 `bool found`（区分 hit/miss/NotFound）
- [x] 17.2 `internal/infra/cache/multi.go`：MultiLevelCache 在 loader 返回 `ErrNotFound` 时缓存 `notFound` 占位（JSON `{"_notFound": true}` + TTL 30s）
- [x] 17.3 `internal/infra/cache/hotspot/redis_hotspot.go`：HotspotLoader 返回 `ErrNotFound` 时缓存 `*notFoundPayload{RefreshedAt: now}`（TTL 30s，与普通数据同 TTL=0 但加 special marker）
- [x] 17.4 统一工具 `internal/infra/cache/notfound.go`：`IsNotFound(err)` / `WrapNotFound(val)` / `UnwrapNotFound(val)` helper
- [x] 17.5 修改 `internal/service/user/blindbox.go` 的 Detail loader：DB 查不到返回 `ErrNotFound`
- [x] 17.6 修改 `internal/service/user/home.go` 的 Feed loader：ES 查不到（不算 NotFound，正常空数组） → 不缓存 NotFound（仅针对"key 本身无效"才缓存）
- [x] 17.7 集成测试：1000 并发请求 NotFound key → DB 只被打 1 次；30s 后占位过期，loader 重新查 DB
- [x] 17.8 集成测试：admin 创建新 blind_box 后 hotspot.Invalidate → 下次读触发冷启动加载真实数据（不会被 NotFound 占位误导）

> 完成备注：`Cache.Get` 三态契约 —— 命中真实值 `(val, true, nil)`、占位命中
> `("", false, nil)`、miss `("", false, ErrCacheMiss)`，Manager 与 MultiLevelCache
> 均在 facade 层做占位值翻译。17.2 的占位写入落在 blindbox Detail 的 L1/L2 路径
> （MultiLevelCache 本身无 loader 概念），TTL 30s；hotspot 层占位用信封
> `not_found` 标记 + `*cache.NotFoundPayload{RefreshedAt}` 载荷，与普通数据一样
> 永驻、随 staleAfter 逻辑过期触发异步重查。`ErrBlindBoxNotFound` 以 `%w`
> 包装 `cache.ErrNotFound`：handler 的 404 映射与缓存层的 IsNotFound 判定
> 双语义同时成立。17.7/17.8 集成测试在 hotspot 包（miniredis）+ service 层
> （blindbox_test 占位流）双覆盖；受影响的 5 个测试 mock 全部适配新签名。

## 18. 缓存预热 + TTL 抖动（对应 D22）

- [ ] 18.1 新增迁移 `000013_mall_user_follow_supplier.{up,down}.sql`：建 mall_user_follow_supplier 表，唯一索引 `(user_id, supplier_id)`
- [ ] 18.2 新增 model：`MallUserFollowSupplier`
- [ ] 18.3 新增 `internal/repository/mall/follow_repository.go`：Follow / Unfollow / ListFollowers / ListFollowing + sqlmock 单测
- [ ] 18.4 新增 `internal/handler/user/follow.go`：POST/DELETE/GET 关注相关接口
- [ ] 18.5 新增 `internal/router/user/router.go` 注册 3 条 follow 路由
- [ ] 18.6 新增 `internal/infra/cache/jitter.go`：`JitterTTL(base time.Duration) time.Duration` helper
- [ ] 18.7 修改所有缓存 TTL 配置：baseTTL + `JitterTTL()` 包装
- [ ] 18.8 新增 `internal/service/preheat/preheat.go`：`PromotionPreheat(ctx, promotion)` / `SeckillPreheat(ctx, seckill)` 异步预热函数
- [ ] 18.9 修改 `internal/service/supplier/promotion.go` 的 Create：事务 COMMIT 后 `go preheat.PromotionPreheat(p)`
- [ ] 18.10 修改 `internal/service/supplier/seckill.go` 的 Create：事务 COMMIT 后 `go preheat.SeckillPreheat(s)`
- [ ] 18.11 前端：用户中心新增"我的关注"页面；盲盒详情页"关注卡商"按钮
- [ ] 18.12 单测：JitterTTL 在范围内；预热失败不影响主业务
