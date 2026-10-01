# trading-card-blindbox-mvp Specification

## Purpose

球星卡盲盒商城 MVP 端到端商业模型：用户登录后从 ES 推荐 feed 浏览盲盒，进入详情查看卡池概率公示与限时特价，一键抽卡（事务内扣库存 + 扣余额 + 落订单 + 抽中卡片），查看订单历史；供应商自助注册后管理自家盲盒与活动；admin 后台管理供应商状态、盲盒状态与推荐位；引入 Redis 缓存详情/feed/限流，引入 Elasticsearch 提供首页推荐。

## Requirements

### Requirement: mall_users field schema

`mall_users` 表 SHALL 在现有 `users` 表基础上扩展（迁移 000006 完成），字段如下：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `username` | varchar(64) | "" | 否 | 用户名（登录用），全局唯一 |
| `password_hash` | varchar(255) | "" | 否 | bcrypt 哈希后的密码 |
| `nickname` | varchar(64) | "" | 否 | 昵称 |
| `avatar` | varchar(255) | "" | 否 | 头像 URL |
| `mobile` | varchar(20) | NULL | 是 | 手机号，全局唯一 |
| `email` | varchar(128) | NULL | 是 | 邮箱，全局唯一 |
| `balance` | decimal(10,2) | 0.00 | 否 | 平台账户余额 |
| `status` | tinyint | 1 | 否 | 1=启用 / 0=禁用 |
| `last_login_at` | datetime(3) | NULL | 是 | 最后登录时间 |
| `last_login_ip` | varchar(45) | "" | 否 | 最后登录 IP |
| `login_failure` | int | 0 | 否 | 累计登录失败次数，达阈值禁用 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

#### Scenario: Seed user login
- **WHEN** seed 创建一条 username="alice", password_hash=bcrypt("alice123"), balance=1000.00 的记录
- **THEN** 用户用 alice / alice123 登录后账户余额显示 1000.00

#### Scenario: Insufficient balance blocks draw
- **WHEN** 用户 balance=10.00 抽一次价格 99.00 的盲盒
- **THEN** 事务回滚，draw_order 不写入，返回 `ErrInsufficientBalance` (HTTP 402)

---

### Requirement: mall_suppliers field schema

`mall_suppliers` 表 SHALL 存储供应商主体（迁移 000007 创建基础字段；迁移 000011 加结算字段）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `name` | varchar(100) | "" | 否 | 供应商名称（用户可见） |
| `logo` | varchar(255) | "" | 否 | 供应商 logo URL |
| `bio` | varchar(500) | "" | 否 | 简介 |
| `contact_phone` | varchar(20) | "" | 否 | 联系电话 |
| `contact_email` | varchar(128) | NULL | 是 | 联系邮箱 |
| `status` | varchar(16) | 'active' | 否 | active / disabled |
| `is_featured` | tinyint(1) | 0 | 否 | 是否推荐位（ES 过滤条件之一） |
| `balance` | decimal(12,2) | 0.00 | 否 | 待结算余额（000011 加） |
| `total_sales` | decimal(12,2) | 0.00 | 否 | 累计销售额（000011 加） |
| `commission_rate` | decimal(5,4) | NULL | 是 | 供应商级手续费率（000011 加；NULL 走全局默认） |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

#### Scenario: Admin disables a supplier
- **WHEN** admin 调用 `POST /api/v1/admin/supplier/:id/toggle_status`，传入 disabled
- **THEN** mall_suppliers.status 变为 'disabled'；该供应商所有盲盒从前端列表消失但不在 DB 级联下架

#### Scenario: Featured supplier is indexed to ES
- **WHEN** 供应商 status=active 且 is_featured=true
- **THEN** `cmd/es-sync` 同步时该供应商被写入 `mall_supplier_index`

---

### Requirement: mall_supplier_users field schema

`mall_supplier_users` 表 SHALL 存储供应商登录账号（迁移 000007）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id，全局唯一索引（一个供应商一个登录账号） |
| `username` | varchar(64) | "" | 否 | 登录用户名，全局唯一 |
| `password_hash` | varchar(255) | "" | 否 | bcrypt 哈希 |
| `status` | tinyint | 1 | 否 | 1=启用 / 0=禁用 |
| `last_login_at` | datetime(3) | NULL | 是 | 最后登录时间 |
| `last_login_ip` | varchar(45) | "" | 否 | 最后登录 IP |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

#### Scenario: Supplier self-registration
- **WHEN** 供应商提交 username="panini_admin", supplier_name="Panini 旗舰店", password="xxx"
- **THEN** mall_suppliers 新增一条 status=active 记录，mall_supplier_users 新增关联登录账号，密码 bcrypt 存储

#### Scenario: Disabled supplier cannot login
- **WHEN** mall_suppliers.status='disabled' 时该 supplier_user 调用登录接口
- **THEN** 返回 `ErrAccountDisabled` (HTTP 403)

---

### Requirement: mall_cards field schema

`mall_cards` 表 SHALL 存储卡牌主数据（迁移 000008）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `name` | varchar(100) | "" | 否 | 卡名（如"勒布朗·詹姆斯 2024 签名卡"） |
| `image` | varchar(255) | "" | 否 | 卡图 URL |
| `team` | varchar(50) | "" | 否 | 球队 |
| `player` | varchar(50) | "" | 否 | 球员 |
| `serial_no` | varchar(50) | "" | 否 | 编号（如"008/100"） |
| `description` | varchar(500) | "" | 否 | 卡描述 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

#### Scenario: Card is reused across pools
- **WHEN** 同一张 mall_card.id=5 同时出现在 mall_card_pool_items 两个不同池里
- **THEN** 数据库允许（card 是主数据，pool_item 是引用）

---

### Requirement: mall_blind_boxes field schema

`mall_blind_boxes` 表 SHALL 存储盲盒 SKU（迁移 000008）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id（强制非空，纯多供应商） |
| `name` | varchar(100) | "" | 否 | 盲盒名 |
| `cover` | varchar(255) | "" | 否 | 封面图 URL |
| `description` | text | "" | 否 | 盲盒描述 |
| `price` | decimal(10,2) | 0.00 | 否 | 原价 |
| `status` | varchar(16) | 'active' | 否 | active / disabled |
| `on_sale` | tinyint(1) | 0 | 否 | 是否在售 |
| `is_featured` | tinyint(1) | 0 | 否 | 是否推荐位 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

索引：`(supplier_id, status, on_sale)`, `(is_featured, status, on_sale)`

#### Scenario: Blind box requires supplier
- **WHEN** 创建 blind_box 时 supplier_id 未指定
- **THEN** 数据库 NOT NULL 约束拒绝写入（HTTP 500 + 错误日志）

#### Scenario: Disabled supplier hides boxes
- **WHEN** mall_suppliers.status='disabled'
- **THEN** 该供应商所有 mall_blind_boxes 在用户端列表中过滤掉（service 层过滤 status=active AND on_sale=true）

---

### Requirement: mall_card_pools field schema

`mall_card_pools` 表 SHALL 1:1 绑定 mall_blind_boxes（迁移 000008）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `blind_box_id` | bigint | 0 | 否 | FK mall_blind_boxes.id，全局唯一索引 |
| `total_draws` | bigint | 0 | 否 | 累计被抽次数（统计用） |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

#### Scenario: Pool auto-created with blind box
- **WHEN** 供应商创建 mall_blind_boxes 时
- **THEN** service 层在同一事务内创建 mall_card_pools（blind_box_id=新 blind_box.id）

---

### Requirement: mall_card_pool_items field schema

`mall_card_pool_items` 表 SHALL 存储池内卡及概率/库存（迁移 000008）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `pool_id` | bigint | 0 | 否 | FK mall_card_pools.id |
| `card_id` | bigint | 0 | 否 | FK mall_cards.id |
| `rarity` | varchar(8) | 'N' | 否 | SSR / SR / R / N |
| `weight` | int | 0 | 否 | 权重，同池加和固定 10000 |
| `stock` | int | 0 | 否 | 剩余库存 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

索引：`(pool_id, stock)`（事务内抽卡时 `WHERE pool_id=? AND stock>0 FOR UPDATE` 走索引）

#### Scenario: Pool weight sums to 10000
- **WHEN** 供应商配置 SSR weight=300, SR weight=1200, R weight=3500, N weight=5000
- **THEN** 抽卡算法以 Σ(weight_i)=10000 为分母，每个 item 命中概率 = weight / Σ

#### Scenario: Stock depletion hides item
- **WHEN** 某 item stock 从 1 减到 0
- **THEN** 后续抽卡 Σ(weight_i) 计算跳过该 item；该 item 不再被命中

#### Scenario: Atomic stock decrement
- **WHEN** 两个并发请求同时抽同一 pool
- **THEN** `SELECT ... FOR UPDATE` 串行化两条事务；后到的事务看到前者的 stock 已减，不会超卖

---

### Requirement: mall_draw_orders field schema

`mall_draw_orders` 表 SHALL 存储抽卡订单（迁移 000009）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `order_no` | varchar(32) | "" | 否 | 订单号（UUID，全局唯一） |
| `user_id` | bigint | 0 | 否 | FK mall_users.id |
| `blind_box_id` | bigint | 0 | 否 | FK mall_blind_boxes.id |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id（冗余，便于按供应商聚合） |
| `price` | decimal(10,2) | 0.00 | 否 | 实际支付价（含活动价） |
| `status` | varchar(16) | 'paid' | 否 | paid / drawn / failed |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

索引：`(user_id, created_at DESC)`（用户订单分页）

#### Scenario: Single draw creates one order
- **WHEN** 用户抽一次
- **THEN** mall_draw_orders 写入一条 status=paid 记录，mall_draw_order_items 写入一条抽中明细（同事务）

---

### Requirement: mall_draw_order_items field schema

`mall_draw_order_items` 表 SHALL 存储订单抽中明细（迁移 000009）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `order_id` | bigint | 0 | 否 | FK mall_draw_orders.id |
| `card_id` | bigint | 0 | 否 | FK mall_cards.id |
| `rarity` | varchar(8) | 'N' | 否 | 抽中时的等级（冗余，避免 join） |
| `snapshot_name` | varchar(100) | "" | 否 | 抽中时卡名快照（防止后续改名影响订单） |
| `snapshot_image` | varchar(255) | "" | 否 | 抽中时卡图快照 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |

#### Scenario: Snapshot immutability
- **WHEN** 用户抽中后管理员修改了 mall_cards.name
- **THEN** 订单详情仍显示抽中时的 snapshot_name（因为是从 draw_order_items 读的快照）

---

### Requirement: mall_promotions field schema

`mall_promotions` 表 SHALL 存储限时特价活动（迁移 000010）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id |
| `blind_box_id` | bigint | 0 | 否 | FK mall_blind_boxes.id |
| `original_price` | decimal(10,2) | 0.00 | 否 | 原价（冗余，便于历史追溯） |
| `promo_price` | decimal(10,2) | 0.00 | 否 | 活动价 |
| `start_at` | datetime(3) | now(3) | 否 | 活动开始时间 |
| `end_at` | datetime(3) | now(3) | 否 | 活动结束时间 |
| `status` | varchar(16) | 'active' | 否 | active / disabled |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

索引：
- 唯一索引：`(blind_box_id, start_at)` —— 同一盲盒时间不重叠
- 普通索引：`(status, end_at)` —— 查"当前生效活动"

#### Scenario: Active promotion applied at draw time
- **WHEN** 用户抽卡时 NOW() 落在某 mall_promotions 的 [start_at, end_at) 内且 status='active'
- **THEN** 抽卡用 promo_price 计费，draw_order.price = promo_price

#### Scenario: Expired promotion reverts to original price
- **WHEN** NOW() > mall_promotions.end_at
- **THEN** 抽卡走 LEFT JOIN 的盲盒原价，draw_order.price = blind_box.price

#### Scenario: Overlapping promotion rejected
- **WHEN** 供应商为同一 blind_box 创建新活动，新 start_at 与已有未结束活动重叠
- **THEN** service 层返回 `ErrPromotionConflict` (HTTP 422)

---

### Requirement: Draw transaction atomicity

抽卡接口 SHALL 在单个数据库事务内完成下列步骤，任一步失败则全部回滚：

1. `SELECT * FROM mall_card_pool_items WHERE pool_id = ? FOR UPDATE`
2. 计算 Σ(weight_i) for stock > 0；若 Σ = 0 → `ErrSoldOut`
3. `rand(0, Σ)` 命中 item
4. `UPDATE mall_card_pool_items SET stock = stock - 1 WHERE id = ? AND stock > 0`（影响行数 = 0 → 重试 1 次）
5. `SELECT price, COALESCE(promo_price, price) AS actual_price FROM mall_blind_boxes bb LEFT JOIN mall_promotions p ...`（同事务）
6. `UPDATE mall_users SET balance = balance - actual_price WHERE id = ? AND balance >= actual_price`（影响行数 = 0 → `ErrInsufficientBalance`）
7. `INSERT mall_draw_orders`
8. `INSERT mall_draw_order_items`

#### Scenario: Successful draw
- **WHEN** 用户 balance=1000、blind_box price=99、pool 有 1 张 SSR weight=300 stock=50
- **THEN** 抽中 SSR，draw_order.price=99（无活动时），draw_order_items 写入 SSR 明细，user balance 变 901，pool item stock 变 49

#### Scenario: Sold out returns error
- **WHEN** pool 所有 item stock=0
- **THEN** 抽卡接口返回 `ErrSoldOut` (HTTP 409)，事务回滚，user balance 与 stock 均不变

#### Scenario: Insufficient balance returns error
- **WHEN** user balance=10、blind_box price=99
- **THEN** 抽卡接口返回 `ErrInsufficientBalance` (HTTP 402)，事务回滚

---

### Requirement: Draw rate limit per user

抽卡接口 SHALL 限制每用户每分钟最多 N 次（MVP N=30）。算法：`SETNX draw:rate:{uid}:{minute} 1 EX 60`，失败 → HTTP 429。

#### Scenario: Within limit allowed
- **WHEN** 用户在 60 秒内抽卡 10 次
- **THEN** 全部成功，第 11 次仍成功（远低于 N=30）

#### Scenario: Exceeds limit blocked
- **WHEN** 用户在 60 秒内抽卡 31 次
- **THEN** 第 31 次返回 HTTP 429 + `code=rate_limit_exceeded`，不进入事务

---

### Requirement: Blind box detail cache

盲盒详情查询接口 SHALL 在查询前先读 Redis `blindbox:detail:{id}`：

- 命中 → 返回缓存
- 未命中 → 查 MySQL（含 LEFT JOIN 当前生效活动）→ 写回 Redis（TTL 10min）→ 返回

#### Scenario: First request populates cache
- **WHEN** 第一次请求 GET /api/v1/user/blindbox/1
- **THEN** DB 查询后 Redis SET blindbox:detail:1 EX 600，第二次请求走缓存

#### Scenario: Edit invalidates cache
- **WHEN** 供应商 PUT /api/v1/supplier/products/1 修改价格
- **THEN** service 层在事务提交后 DEL blindbox:detail:1，下次请求重新加载

---

### Requirement: Home feed via Elasticsearch

首页 feed 接口 SHALL 走 ES 查询（不直查 MySQL）：

- 索引 `mall_blind_box_index`：过滤 `status=active AND on_sale=true AND is_featured=true`，按 `hot_score DESC, created_at DESC` 排序
- 索引 `mall_supplier_index`：过滤 `status=active AND is_featured=true`，按 `featured_rank ASC, blind_box_count DESC` 排序
- 应用层只读 ES，文档形态是"投影"（见 design D7）
- ES 不可用 → HTTP 500 + `code=search_unavailable`（不 fallback）

#### Scenario: ES query returns featured items
- **WHEN** GET /api/v1/user/home/feed 调用
- **THEN** 响应包含 featured 盲盒列表 + featured 供应商列表，按上述规则排序

#### Scenario: ES unavailable fails fast
- **WHEN** ES 连接超时
- **THEN** 接口返回 HTTP 500 + `code=search_unavailable`，不查 MySQL 兜底

---

### Requirement: ES sync script (cmd/es-sync)

`cmd/es-sync` 入口 SHALL 全量同步：

- 读 MySQL：盲盒 `WHERE status=active AND on_sale=true AND is_featured=true`，供应商 `WHERE status=active AND is_featured=true`
- 转 ES 投影（见 design D7）
- delete + bulk index 全量覆盖 ES 索引
- 输出同步统计：`mall_blind_box_index: N records synced in M ms` / `mall_supplier_index: ...`

#### Scenario: Idempotent re-run
- **WHEN** 同一脚本连续运行两次
- **THEN** 第二次运行不产生重复记录（delete 先清空）

---

### Requirement: Configuration for Redis and ES

新增两个配置文件：

`config/cache.yaml`：
```yaml
redis:
  host: 127.0.0.1
  port: 6379
  password: ""
  db: 0
  pool_size: 50
  dial_timeout: 5s
```

`config/search.yaml`：
```yaml
elasticsearch:
  addresses:
    - http://127.0.0.1:9200
  username: ""
  password: ""
  index_prefix: mall
  request_timeout: 10s
```

#### Scenario: App fails fast on Redis unavailable
- **WHEN** 启动期 Redis 不可达
- **THEN** cmd/serve 启动失败，log 输出连接错误，进程退出码非零

#### Scenario: App fails fast on ES unavailable
- **WHEN** 启动期 ES 不可达
- **THEN** cmd/serve 启动失败（同上）

---

### Requirement: Cache TTL configuration

缓存 TTL SHALL 在 `config/cache.yaml` 中可配置：

```yaml
ttl:
  blindbox_detail: 10m
  home_feed: 5m
  rate_limit: 60s
```

#### Scenario: TTL change takes effect
- **WHEN** 配置 blindbox_detail=20m
- **THEN** 重启后所有 `blindbox:detail:*` key TTL 均为 20 分钟

---

### Requirement: mall_suppliers settlement fields extension

`mall_suppliers` 表 SHALL 在原 schema 基础上扩展（迁移 000011）以下结算相关字段：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `balance` | decimal(12,2) | 0.00 | 否 | 待结算余额（抽卡入 +，mark_paid 出 -） |
| `total_sales` | decimal(12,2) | 0.00 | 否 | 累计销售额（仅递增） |
| `commission_rate` | decimal(5,4) | NULL | 是 | 供应商级手续费率（如 0.1000=10%）；NULL 时使用全局默认 |

索引：(status, is_featured) 已有，无需新增。

#### Scenario: Supplier balance increases on draw
- **WHEN** 抽卡事务成功扣减 user balance += actual_price
- **THEN** 同事务内 supplier.balance += actual_price，total_sales += actual_price

#### Scenario: NULL commission_rate uses default
- **WHEN** supplier.commission_rate = NULL
- **THEN** 结算单生成时使用 config.yaml 的 default_commission_rate（如 0.1000）

---

### Requirement: mall_settlements field schema

`mall_settlements` 表 SHALL 存储结算单（迁移 000011）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id |
| `period_start` | datetime(3) | now(3) | 否 | 周期开始（含） |
| `period_end` | datetime(3) | now(3) | 否 | 周期结束（不含） |
| `total_amount` | decimal(12,2) | 0.00 | 否 | 周期内销售总额 |
| `commission_rate` | decimal(5,4) | 0.0000 | 否 | 当时手续费率快照 |
| `commission_amount` | decimal(12,2) | 0.00 | 否 | 平台手续费 |
| `payout_amount` | decimal(12,2) | 0.00 | 否 | 应付供应商 |
| `status` | varchar(16) | 'pending' | 否 | pending / processing / paid / failed |
| `paid_at` | datetime(3) | NULL | 是 | 标记打款时间 |
| `paid_by` | bigint | NULL | 是 | 标记打款的 admin id |
| `remark` | varchar(255) | "" | 否 | 备注 / 银行流水号 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

索引：
- 唯一索引：`(supplier_id, period_start, period_end)` —— 防止同一周期重复结算
- 普通索引：`(status, created_at)` —— 列表分页

#### Scenario: Settlement generated from pending orders
- **WHEN** admin POST /admin/settlements {supplier_id, period_start, period_end}
- **THEN** service 聚合该供应商该周期内所有 status=paid 且未被结算的订单，生成 settlement + items，写入 pending 状态

#### Scenario: Empty period returns error
- **WHEN** 周期内没有 paid 订单
- **THEN** 返回 `ErrNoOrdersToSettle` (HTTP 422)，不写入 settlement

#### Scenario: Overlapping settlement rejected
- **WHEN** 已有 (supplier_id=1, period_start=2026-09-01, period_end=2026-10-01) 的 settlement
- **THEN** 再创建重叠区间的 settlement 返回 `ErrSettlementConflict` (HTTP 422)

---

### Requirement: mall_settlement_items field schema

`mall_settlement_items` 表 SHALL 存储结算单关联订单明细（迁移 000011）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `settlement_id` | bigint | 0 | 否 | FK mall_settlements.id |
| `draw_order_id` | bigint | 0 | 否 | FK mall_draw_orders.id，全局唯一索引（一订单只可被结算一次） |
| `amount` | decimal(10,2) | 0.00 | 否 | 该订单销售额（=draw_order.price） |
| `commission_amount` | decimal(10,2) | 0.00 | 否 | 该订单分摊的平台手续费 |
| `payout_amount` | decimal(10,2) | 0.00 | 否 | 该订单分摊的应付供应商 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |

索引：`(settlement_id)`, `(draw_order_id) UK`

#### Scenario: One draw order in exactly one settlement
- **WHEN** draw_order.id=100 被 settlement.id=5 包含
- **THEN** 后续任何结算尝试包含该订单 → ErrOrderAlreadySettled（draw_order_id 唯一索引拒绝）

---

### Requirement: mall_platform_ledger schema

`mall_platform_ledger` 表 SHALL 存储平台收入台账聚合行（迁移 000011）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `counter_key` | varchar(32) | "" | 否 | 主键，聚合指标名（如 `total_revenue` / `total_commission`） |
| `amount` | decimal(14,2) | 0.00 | 否 | 当前累计值 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |

#### Scenario: Total revenue counter accumulates
- **WHEN** 抽卡事务成功
- **THEN** `INSERT INTO mall_platform_ledger (counter_key='total_revenue', amount=actual_price) ON DUPLICATE KEY UPDATE amount=amount+VALUES(amount)` 累加

#### Scenario: Commission counter on mark_paid
- **WHEN** admin 标记 settlement 为 paid
- **THEN** 同事务内 `total_commission` counter += settlement.commission_amount

---

### Requirement: Draw transaction settlement side-effects

抽卡事务（D3）成功后 SHALL 在同一事务内更新结算相关字段（步骤 7.5 / 7.6）：

1. `UPDATE mall_suppliers SET balance = balance + actual_price, total_sales = total_sales + actual_price WHERE id = ?`
2. `INSERT INTO mall_platform_ledger (counter_key='total_revenue', amount=actual_price) ON DUPLICATE KEY UPDATE amount=amount+VALUES(amount)`

任一失败 → 整个事务回滚。

#### Scenario: Draw succeeds, supplier balance updated atomically
- **WHEN** 用户抽卡事务成功
- **THEN** user.balance -= actual_price，supplier.balance += actual_price，platform_ledger.total_revenue += actual_price 三者同事务原子

#### Scenario: Draw rolls back all settlement side-effects
- **WHEN** 步骤 7（扣 user balance）影响行数=0
- **THEN** 步骤 7.5 / 7.6 未执行（事务回滚），supplier balance 与 platform ledger 不变

---

### Requirement: Settlement generation (manual)

admin SHALL 能通过 `POST /api/v1/admin/settlements` 手动生成结算单：

入参：`{supplier_id, period_start, period_end}`

处理（事务内）：
1. 校验供应商存在 + status=active
2. 校验 (supplier_id, period_start, period_end) 唯一
3. 查 draw_orders：`status=paid AND created_at IN [start, end) AND id NOT IN settlement_items`，FOR UPDATE
4. 计算 total / commission / payout
5. INSERT settlement (status=pending) + batch INSERT items

#### Scenario: Valid generation creates settlement
- **WHEN** admin 生成 2026-09-01 ~ 2026-10-01 周期、supplier_id=1 的结算单，期内有 10 笔订单共 990 元
- **THEN** 创建 settlement: total=990, commission_rate=0.10, commission=99, payout=891，10 条 items

#### Scenario: Preview without writing
- **WHEN** admin 调用 `POST /admin/settlements/preview` 同样参数
- **THEN** 返回预览数据（不写库）：{order_count, total, commission, payout}

---

### Requirement: Settlement mark_paid

admin SHALL 能通过 `POST /api/v1/admin/settlements/:id/mark_paid` 标记结算单已打款：

入参：`{payment_ref, remark}`

处理（事务内）：
1. 校验 settlement.status='pending'
2. UPDATE settlement: status='paid', paid_at=NOW(), paid_by=?, remark=?
3. UPDATE mall_suppliers: balance -= payout_amount（条件 balance >= payout_amount，影响行数=0 → ErrInsufficientSupplierBalance）
4. UPDATE mall_platform_ledger: total_commission += commission_amount

#### Scenario: Mark paid successfully
- **WHEN** settlement.status='pending' 且 supplier.balance >= payout_amount
- **THEN** settlement.status='paid'，supplier.balance -= payout_amount，platform_ledger.total_commission += commission_amount

#### Scenario: Mark paid with insufficient supplier balance
- **WHEN** supplier.balance < payout_amount（理论上不应发生，但防御性检查）
- **THEN** 返回 `ErrInsufficientSupplierBalance` (HTTP 409)，settlement 不更新

#### Scenario: Double mark_paid rejected
- **WHEN** settlement.status='paid'
- **THEN** 再调用 mark_paid 返回 `ErrSettlementAlreadyPaid` (HTTP 409)

---

### Requirement: mall_seckill_activities field schema

`mall_seckill_activities` 表 SHALL 存储秒杀活动主体（迁移 000012）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id |
| `blind_box_id` | bigint | 0 | 否 | FK mall_blind_boxes.id |
| `seckill_price` | decimal(10,2) | 0.00 | 否 | 秒杀价 |
| `total_stock` | int | 0 | 否 | 秒杀总名额（用于初始化 Redis 库存） |
| `per_user_limit` | int | 1 | 否 | 每用户限购数量 |
| `start_at` | datetime(3) | now(3) | 否 | 活动开始时间 |
| `end_at` | datetime(3) | now(3) | 否 | 活动结束时间 |
| `status` | varchar(16) | 'active' | 否 | active / disabled |
| `redis_initialized` | tinyint(1) | 0 | 否 | Redis 库存是否已初始化 |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `updated_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

索引：`(status, start_at, end_at)` —— 查"当前生效的秒杀活动"

#### Scenario: Seckill activity creation initializes Redis
- **WHEN** 供应商创建 seckill 活动，total_stock=100，per_user_limit=1
- **THEN** 同一事务内 SET seckill:stock:{new_id} = 100 + UPDATE redis_initialized=true；DB 与 Redis 一致

#### Scenario: Seckill rejects when card pool stock insufficient
- **WHEN** 创建秒杀活动时 mall_card_pool_items 总 stock < total_stock
- **THEN** 返回 `ErrSeckillStockExceedsPool` (HTTP 422)，DB 不写入

---

### Requirement: mall_stock_deduction_log field schema

`mall_stock_deduction_log` 表 SHALL 存储秒杀路径下的库存扣减记录（迁移 000012）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `seckill_id` | bigint | 0 | 否 | FK mall_seckill_activities.id |
| `user_id` | bigint | 0 | 否 | FK mall_users.id |
| `blind_box_id` | bigint | 0 | 否 | FK mall_blind_boxes.id |
| `card_id` | bigint | 0 | 否 | FK mall_cards.id（抽中的卡） |
| `rarity` | varchar(8) | 'N' | 否 | 抽中等级 |
| `snapshot_name` | varchar(100) | "" | 否 | 卡名快照 |
| `snapshot_image` | varchar(255) | "" | 否 | 卡图快照 |
| `deducted_at` | datetime(3) | now(3) | 否 | 扣减时间 |
| `synced_to_pool_at` | datetime(3) | NULL | 是 | 异步对账完成时间（NULL = 未对账） |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |

索引：
- `(synced_to_pool_at, deducted_at)` —— 异步任务扫"待对账"
- `(seckill_id)` —— 按活动聚合

#### Scenario: Seckill draw writes deduction log
- **WHEN** 用户秒杀下单成功
- **THEN** mall_stock_deduction_log 新增一条 synced_to_pool_at=NULL 记录

#### Scenario: Async sync marks log as synced
- **WHEN** `cmd/stock-sync` 跑批
- **THEN** 把 synced_to_pool_at IS NULL 的记录标记 synced_to_pool_at=NOW()（秒杀路径下卡池库存已在事务内扣过，此处仅标记对账）

---

### Requirement: Seckill draw flow (Redis pre-deduct + transaction + rollback)

秒杀下单接口 SHALL 按 design D15 流程执行：

1. 预校验：seckill.status='active' AND redis_initialized=true AND NOW() IN [start_at, end_at)
2. 限购校验：`INCR seckill:user_bought:{sid}:{uid}`，> per_user_limit → DECR 回滚 → `ErrUserLimitExceeded`
3. Redis 库存预扣：`DECR seckill:stock:{sid}`，< 0 → INCR + DECR user_bought 回滚 → `ErrSoldOut`
4. MySQL 事务（轻量）：
   - 抽卡算法（同 D3）：SELECT pool items FOR UPDATE + rand + UPDATE stock -= 1
   - INSERT mall_stock_deduction_log
   - INSERT mall_draw_orders (source='seckill')
   - INSERT mall_draw_order_items (snapshot_*)
   - UPDATE mall_users balance -= seckill_price（条件更新）
   - UPDATE mall_suppliers balance += seckill_price + ledger（结算副作用）
5. 失败：ROLLBACK + Redis INCR 回滚 + DECR user_bought 回滚

#### Scenario: Seckill success returns card
- **WHEN** 用户秒杀下单，Redis 库存充足，事务成功
- **THEN** 返回 draw_order + draw_order_items（抽中的卡）；Redis seckill:stock 减 1；user balance 减 seckill_price；stock_deduction_log 写入

#### Scenario: Seckill sold out via Redis pre-deduct
- **WHEN** Redis DECR 后 remaining < 0
- **THEN** 立即 INCR 回滚 + DECR user_bought 回滚；返回 `ErrSoldOut` (HTTP 409)；MySQL 不写入

#### Scenario: MySQL failure rolls back Redis
- **WHEN** 步骤 4 事务失败（余额不足 / 库存不足）
- **THEN** ROLLBACK + Redis INCR + DECR user_bought；返回对应错误码；前端提示重试

---

### Requirement: Stock-sync script (cmd/stock-sync)

`cmd/stock-sync` SHALL 每分钟（或手动）执行：

1. 查 `mall_stock_deduction_log WHERE synced_to_pool_at IS NULL ORDER BY deducted_at LIMIT 1000`
2. 批量 UPDATE `synced_to_pool_at = NOW()`
3. 输出统计：`synced_count` / `synced_pools` / `elapsed_ms`

注意：秒杀路径下 `mall_card_pool_items.stock` 已在事务内扣过（D15 步骤 4.b），本脚本**仅做对账标记**，不实际变更库存。

#### Scenario: Idempotent re-run
- **WHEN** stock-sync 连续跑两次
- **THEN** 第二次扫到的行数=0（因第一次已全部标记 synced）

---

### Requirement: Multi-level cache architecture

缓存层 SHALL 实现 L1（进程内 5-30s）+ L2（Redis 5min+）+ L3（MySQL）三层架构：

- `internal/infra/cache/cache.go` 定义 `Cache` 接口（Get / Set / Del / SetNX）
- `internal/infra/cache/memory.go` L1 内存实现（`github.com/patrickmn/go-cache`）
- `internal/infra/cache/redis.go` L2 Redis 实现
- `internal/infra/cache/multi.go` `MultiLevelCache` 组合：Get 按 L1→L2→DB，回写 + 反向失效
- 业务代码只调 `MultiLevelCache`，不感知层级

#### Scenario: Read flow with cache hit at L1
- **WHEN** 第一次 Get('blindbox:detail:1') → miss L1 + miss L2 + 查 DB + 回填 L2+L1
- **THEN** 第二次 Get 同 key → L1 命中返回（≤ 10s TTL）

#### Scenario: Read flow with cache hit at L2 only
- **WHEN** L1 TTL 到期或被清空
- **THEN** 第二次 Get → miss L1 + hit L2 → 回填 L1 → 返回（无需查 DB）

#### Scenario: Write flow invalidates both layers
- **WHEN** 供应商编辑盲盒（事务提交后）
- **THEN** MultiLevelCache.Del('blindbox:detail:{id}') → DEL L2 (Redis) + DEL L1 (memory)

#### Scenario: Rate limit bypasses L1
- **WHEN** 抽卡限流 SETNX
- **THEN** 走 Redis 直连，不走 MultiLevelCache（L1 不能缓存限流状态）

#### Scenario: L1 cross-process eventual consistency
- **WHEN** 进程 A 写了新值（DEL L1 + DEL L2 + DB 更新），进程 B 仍有旧 L1 值
- **THEN** 进程 B 在 ≤ 30s 内（最长 L1 TTL）会自然过期，下次 Get 查 L2 拿新值

---

### Requirement: MQ multi-driver abstraction

`internal/infra/mq/` SHALL 提供 MQ 抽象：

```go
type MQ interface {
    Publish(ctx context.Context, topic string, payload []byte) error
    Subscribe(ctx context.Context, topic, consumerGroup string, handler func(payload []byte) error) error
}
```

MVP SHALL 仅实现 Redis Stream driver。config.yaml 通过 `mq.driver: redis` 切换；未来加 Kafka 不动业务代码。

#### Scenario: Publish succeeds returns nil
- **WHEN** 调用 `mq.Publish(ctx, "stock.deduction.sync", payload)`
- **THEN** Redis `XADD` 返回 message id，无错返回 nil

#### Scenario: Subscribe processes incoming messages
- **WHEN** `cmd/stock-sync` 启动并 `Subscribe(ctx, "stock.deduction.sync", "stock-sync-group", handler)`
- **THEN** handler 被持续调用；handler 返回 nil → `XACK`；handler 返回 err → 消息保留在 pending list

---

### Requirement: Seckill transaction publishes MQ event

秒杀事务提交后 SHALL 异步 PUBLISH 一条消息到 `stock.deduction.sync` topic：

```
tx.Commit()
  ↓
go mq.Publish(ctx, "stock.deduction.sync", json.Marshal({seckill_id, log_id, user_id, ...}))
```

publish 失败 SHALL 仅记录 warn 日志，不影响业务主路径（cron 兜底）。

#### Scenario: Successful draw publishes message
- **WHEN** 秒杀抽卡事务成功
- **THEN** goroutine 异步调用 mq.Publish，MQ 中多一条 `stock.deduction.sync` 消息

#### Scenario: Publish failure does not rollback transaction
- **WHEN** mq.Publish 返回 err（Redis 不可达）
- **THEN** 事务不回滚（已提交）；warn 日志；cmd/stock-sync cron 兜底扫 synced_to_pool_at IS NULL

---

### Requirement: cmd/stock-sync consumer mode

`cmd/stock-sync` SHALL 作为消费者进程运行，订阅 `stock.deduction.sync` topic：

- handler 收到消息 → 解析 seckill_id / log_id → UPDATE `mall_stock_deduction_log SET synced_to_pool_at=NOW() WHERE id=?`
- 失败：handler 返回 err → 不 ack → 下次重试

#### Scenario: Consumer processes message
- **WHEN** 新消息到达 `stock.deduction.sync` topic
- **THEN** handler 在 ≤ 100ms 内处理；UPDATE 写入 synced_to_pool_at=NOW()

#### Scenario: Consumer failure retains pending
- **WHEN** handler 返回 err（DB 异常）
- **THEN** 消息保留在 pending list，下次 `XREADGROUP` 重试

---

### Requirement: cmd/stock-sync cron fallback

`cmd/stock-sync` SHALL 同时支持 cron 兜底模式（每 5 分钟）：

```
for {
    SELECT * FROM mall_stock_deduction_log
    WHERE synced_to_pool_at IS NULL
    ORDER BY deducted_at LIMIT 1000

    if rows > 0:
        batch UPDATE synced_to_pool_at=NOW()
    else:
        sleep 5min
}
```

#### Scenario: Cron fallback processes leftovers
- **WHEN** MQ 失败导致部分 log 未同步
- **THEN** cron 兜底每 5 分钟扫一次，最终全部同步

#### Scenario: Consumer + cron run together without conflict
- **WHEN** consumer 已处理消息 + cron 扫到同一行
- **THEN** consumer 的 UPDATE 在前；cron 扫到的是 synced_to_pool_at IS NOT NULL，UPDATE 影响行数=0，幂等无副作用

---

### Requirement: Third-party channel adapter and factory

`internal/infra/channel/` SHALL 提供第三方渠道抽象：

```go
type Channel interface {
    Pay(ctx context.Context, req *PayRequest) (*PayResult, error)
    Payout(ctx context.Context, req *PayoutRequest) (*PayoutResult, error)
    Notify(ctx context.Context, req *NotifyRequest) error
}

type ChannelFactory interface {
    Create(name string) (Channel, error)  // name = "payment" / "bank" / "sms"
}
```

MVP SHALL 仅实现 `mock` driver（返回成功 + 记录调用日志）。未来添加 wechat/alipay/real-bank driver 不动业务代码。

config/channel.yaml 通过 `channel.payment: mock` 等字段指定每种渠道的 driver。

#### Scenario: Factory returns mock channel
- **WHEN** 业务代码调 `factory.Create("payment")`，config `channel.payment=mock`
- **THEN** 返回 mock 实现；`Pay()` 返回 `PayResult{Status: "success"}`

#### Scenario: Unknown channel name rejected
- **WHEN** `factory.Create("unknown")` 调用
- **THEN** 返回 `ErrUnknownChannel`（业务层处理为 500 + 错误日志）

---

### Requirement: Draw order state machine

`internal/domain/state/` SHALL 提供抽卡订单状态机实现：

| 当前状态 | 合法目标状态 |
|---|---|
| pending | paid, failed |
| paid | drawn, failed |
| drawn | （终态） |
| failed | （终态） |

State 接口 SHALL 包含 `Name()` / `CanTransitionTo(target)` / `OnEnter(ctx, order)` 三个方法。

业务 service SHALL 在状态变更时调用 `state.Transition(newState)`，自动校验合法性 + 触发 OnEnter hook + 持久化到 DB。

#### Scenario: Valid transition pending → paid
- **WHEN** 抽卡事务成功，order 从 pending 转 paid
- **THEN** `CanTransitionTo(PaidState)` 返回 true；`Transition(PaidState)` 成功；DB status='paid'

#### Scenario: Invalid transition pending → drawn rejected
- **WHEN** 抽卡事务中跳过 paid 直接转 drawn
- **THEN** `CanTransitionTo(DrawnState)` 返回 false；返回 `ErrInvalidStateTransition` (HTTP 409)

#### Scenario: Terminal state cannot transition
- **WHEN** order.status='drawn'（终态）
- **THEN** 任何 `Transition(...)` 调用返回 `ErrInvalidStateTransition`

---

### Requirement: Settlement order state machine

结算单状态机 SHALL 按下表规则：

| 当前状态 | 合法目标状态 |
|---|---|
| pending | processing, failed |
| processing | paid, failed |
| paid | （终态） |
| failed | （终态） |

#### Scenario: Valid transition pending → processing
- **WHEN** admin 生成结算单（status='pending'）后开始处理
- **THEN** Transition(ProcessingState) 成功；DB status='processing'

#### Scenario: Valid transition processing → paid
- **WHEN** admin mark_paid 完成
- **THEN** Transition(PaidState) 成功；DB status='paid'；supplier balance 扣减

---

### Requirement: Draw validation chain (responsibility chain)

`internal/domain/chain/` SHALL 提供责任链编排：

```go
type Validator interface {
    Name() string
    Validate(ctx context.Context, input *DrawInput) error
}

type Chain struct{ validators []Validator }

func (c *Chain) Validate(ctx, input) error  // 顺序执行，任一 err 终止
```

MVP SHALL 提供以下 Validator：

| Validator | 检查项 | 失败错误码 |
|---|---|---|
| UserActiveCheck | mall_users.status=1 | ErrUserDisabled (403) |
| BlindBoxBuyableCheck | blind_box.status=active AND on_sale AND supplier.status=active | ErrBlindBoxNotBuyable (404) |
| TimeWindowCheck | 普通活动 NOW() IN [start_at, end_at) / 秒杀活动同理 | ErrNotInTimeWindow (409) |
| UserLimitCheck（秒杀） | Redis INCR seckill:user_bought <= per_user_limit | ErrUserLimitExceeded (429) |
| BalanceCheck | user.balance >= actual_price | ErrInsufficientBalance (402) |

Chain SHALL 在事务外执行（不进事务），校验失败 fast return 不浪费 DB 连接。

#### Scenario: All checks pass
- **WHEN** 抽卡请求，所有 Validator 顺序通过
- **THEN** Chain.Validate 返回 nil；service 进入事务执行抽卡算法

#### Scenario: Balance insufficient fails fast
- **WHEN** user.balance=10, price=99
- **THEN** BalanceCheck.Validate 返回 ErrInsufficientBalance；Chain 立即终止；事务未开启

#### Scenario: Seckill chain adds UserLimitCheck
- **WHEN** 秒杀路径下，user 已购 5 次，per_user_limit=3
- **THEN** UserLimitCheck 返回 ErrUserLimitExceeded；INCR 已回滚

#### Scenario: New check inserted without modifying existing
- **WHEN** 新增 RiskCheck（如黑名单检查）
- **THEN** 仅在 DrawChain.validators 列表中插入；其他 Validator 零改动

---

### Requirement: Hotspot cache (no TTL + logical time + distributed lock + async refresh)

`internal/infra/cache/hotspot/` SHALL 提供热点数据缓存：

```go
type HotspotLoader func(ctx context.Context, key string) (data any, err error)

type HotspotCache interface {
    Get(ctx context.Context, key string, loader HotspotLoader, staleAfter time.Duration) (data any, err error)
    Invalidate(ctx context.Context, key string) error
}
```

**算法**（Redis 实现）：
1. `GET key` → val；nil → 冷启动 → loader 查 DB → `SET key val EX 0`（不过期）→ 返回
2. val 不 nil → 解析 `RefreshedAt`；now - RefreshedAt > staleAfter → 抢分布式锁
3. 抢到锁 → `go` 异步 loader + SET；未抢到 → 直接返回老数据
4. 不管抢没抢到，都返回当前 val（老数据）

**分布式锁**：`SET lock:{key} {uuid} NX EX 5`；释放用 Lua 脚本只删自己的锁。

**singleflight 合并**：冷启动路径用 `golang.org/x/sync/singleflight.Do`，避免 N 个并发请求同时打 DB。

**应用场景**：

| Key 模式 | staleAfter | 用途 |
|---|---|---|
| `hotspot:blindbox:{id}` | 30s | 热门盲盒详情 |
| `hotspot:feed:home` | 60s | 首页 feed |

#### Scenario: Cold start loads from DB
- **WHEN** 首次请求 `hotspot:blindbox:1`，Redis 无该 key
- **THEN** loader 查 DB → SET key val EX 0（不过期）→ 返回数据

#### Scenario: Logical time triggers async refresh
- **WHEN** 第二次请求，data.RefreshedAt 距今 > 30s
- **THEN** 抢分布式锁 → 抢到者 `go` 异步 loader 查 DB 刷新 cache；当前请求返回老数据

#### Scenario: Concurrent requests see stale data without lock contention
- **WHEN** 100 个并发请求都发现 staleAfter 到期
- **THEN** 1 个抢到锁的请求异步刷新；99 个未抢到锁的请求直接返回老数据（无 DB 压力）

#### Scenario: Active invalidation forces reload
- **WHEN** 供应商编辑盲盒，service 调 `hotspot.Invalidate("hotspot:blindbox:1")`
- **THEN** Redis DEL key；下次请求触发冷启动 → loader 查 DB → 写回

#### Scenario: Lock release safety via Lua script
- **WHEN** 持锁者 goroutine panic / 超时（5s TTL 自动释放）
- **THEN** Lua 脚本保证新持锁者能正确获取；不会误删别人的锁

---

### Requirement: Cache penetration protection (negative cache)

所有缓存层（L1/L2 MultiLevelCache + Hotspot）SHALL 实现穿透防护：

- DB 查询返回 `ErrNotFound` 时，缓存一个 `notFound` 占位（TTL 短：30s-60s）
- 后续相同 key 请求直接命中占位，返回 `ErrNotFound`，**不查 DB**
- 业务写入时主动 invalidate 占位（让真实数据能被加载）
- L1/L2 用 JSON `{"_notFound": true}` 标记；Hotspot 用 `*notFoundPayload{RefreshedAt: now}`

**NotFound TTL 选择**：
- 30s（默认）：平衡"穿透防护"与"新数据可被发现"的窗口
- 60s（保守）：防护更强，但新数据要 60s 后才可见

#### Scenario: Non-existent key returns NotFound from cache
- **WHEN** 第一次请求 `blindbox:detail:99999`（盲盒不存在），loader 返回 `ErrNotFound`
- **THEN** 缓存写入 `notFound` 占位（TTL 30s）；第二次相同请求直接返回 `ErrNotFound`，不打 DB

#### Scenario: Penetration attack blocked
- **WHEN** 1000 并发请求查询不存在的 key
- **THEN** 第 1 次 loader 查 DB 返回 NotFound；后续 999 次命中占位；DB 只被打 1 次

#### Scenario: New data invalidates NotFound placeholder
- **WHEN** admin 创建 blind_box id=99999（之前不存在），调 `hotspot.Invalidate("hotspot:blindbox:99999")`
- **THEN** NotFound 占位被 DEL；下次读触发冷启动 → loader 查 DB 返回真实数据 → 写回 cache

#### Scenario: NotFound TTL expires and re-loads
- **WHEN** 30s 后再次请求之前 NotFound 的 key（数据仍不存在）
- **THEN** TTL 到位失效；走冷启动路径 → loader 查 DB → 重新写入新占位

---

### Requirement: mall_user_follow_supplier field schema

`mall_user_follow_supplier` 表 SHALL 存储用户关注供应商关系（迁移 000013）：

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `user_id` | bigint | 0 | 否 | FK mall_users.id |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

唯一索引：`(user_id, supplier_id)`

#### Scenario: User follows supplier
- **WHEN** user 调用 `POST /api/v1/user/follow/suppliers/10`
- **THEN** mall_user_follow_supplier 写入一条 user_id=uid, supplier_id=10 记录

#### Scenario: Idempotent follow
- **WHEN** 重复关注同一供应商
- **THEN** 唯一索引拒绝，返回 `ErrAlreadyFollowed` (HTTP 409)

---

### Requirement: Cache preheat on promotion/seckill creation

创建限时特价 / 秒杀活动成功后 SHALL 异步预热相关缓存：

**预热对象**：
1. 该供应商的所有 active blind_box 详情（`hotspot:blindbox:{id}`）
2. 关注该供应商的所有用户的首页 feed（`hotspot:feed:user:{uid}`）

**触发方式**：事务 COMMIT 后 `go preheat.PromotionPreheat(promotion)`（异步，不阻塞主业务）。

**失败处理**：预热 goroutine 失败仅记 warn 日志，不影响活动创建结果。

#### Scenario: Promotion created triggers preheat
- **WHEN** 供应商创建限时特价活动成功
- **THEN** 异步预热：
  - 该 supplier_id 下所有 active blind_box 的详情
  - 关注该 supplier_id 的所有用户的 feed
  - 不阻塞活动创建响应

#### Scenario: Seckill created triggers preheat
- **WHEN** 供应商创建秒杀活动成功
- **THEN** 同上预热逻辑

#### Scenario: Preheat failure does not rollback creation
- **WHEN** 预热 goroutine 抛 panic 或 loader 报错
- **THEN** 活动创建事务不回滚（已 COMMIT）；warn 日志；下次正常请求触发 cold start 补齐

---

### Requirement: TTL random jitter

所有缓存 TTL SHALL 经过 `JitterTTL(base)` 处理，添加 +0%~+25% 的随机抖动：

```go
func JitterTTL(base time.Duration) time.Duration {
    return base + time.Duration(rand.Int63n(int64(base)/4))
}
```

#### Scenario: TTL within jitter range
- **WHEN** 配置 baseTTL=30s
- **THEN** 实际 TTL 在 [30s, 37.5s) 之间随机

#### Scenario: Jitter prevents synchronized expiration
- **WHEN** 1000 个 key 同时写入（baseTTL=10min）
- **THEN** 实际 TTL 在 [10min, 12.5min) 之间均匀分布；不会出现"全部同时过期"
