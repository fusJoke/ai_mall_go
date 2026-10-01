# Design: 球星卡盲盒商城 MVP

See `proposal.md` for motivation and `specs/trading-card-blindbox-mvp/spec.md` for the requirement set. This document only covers architectural decisions.

## Context

仓库现状：

- 后端：admin 完整闭环（init/manager/rule/group/log/upload/auth/captcha）+ 5 个 migrations 已落地。
- 前端：Vue 3 + Vue Router + i18n，已有 `layouts/admin/` 完整结构；`views/admin/` 含 dashboard/group/log/manager/rule/login；`views/user/` 与 `views/supplier/` 不存在。
- token infra：`internal/infra/token/driver/database.go` 已落地，注释预留"后续可加 redis"。
- ES / Redis：当前仓库不依赖。
- 业务代码：除 admin 外为零；`model/user.go` 是 AutoMigrate 演示占位模型。

MVP 目标：在「概率公示 + 随机抽取 + 多供应商入驻 + 限时特价」商业模型下，端到端打通 C 端抽卡主流程，引入 Redis 缓存层与 ES 推荐层作为规模化基础设施。

## Goals / Non-Goals

### Goals

- **端到端主流程**：用户登录 → 首页 feed（ES）→ 盲盒详情 → 抽卡（事务 + 并发安全）→ 看订单。
- **多供应商入驻**：供应商自助注册（免审），可管理自家盲盒与活动。
- **admin 后台扩展**：可管理供应商状态、盲盒状态、推荐位。
- **限时特价**：供应商可对自家盲盒设置活动价 + 起止时间，过期自动回原价。
- **缓存 + 推荐**：盲盒详情缓存、首页 feed 缓存、抽卡限流、ES 首页 feed。
- **分层严格遵循**：Handler → Service → Repository → Model；身份子目录 `admin/` / `user/` / `supplier/`。

### Non-Goals

- 用户注册流程 / 验证码
- 真实支付 / 退款 / 抽卡回滚
- 收藏 / 晒卡 / 集卡册 / 抽卡保底
- 卡牌二级交易
- 供应商资质审核流
- ES 实时同步（CDC / binlog）
- token driver 切换 Redis
- 库存计数器缓存 / 用户余额缓存
- 多 ES 集群 / 多 Redis 哨兵
- 满减 / 赠品 / 会员折扣等多活动类型
- 活动叠加 / 互斥规则

## Decisions

### D1：身份体系三分（user + supplier + admin），三套独立 token

- `user/` 用户端 C 端登录（手机号 + 密码，MVP 不做注册）
- `supplier/` 供应商端 B 端登录（用户名 + 密码）
- `admin/` 后台登录（已有）
- 三套 token 通过 `internal/infra/token/` 现有 driver 机制管理，仅 `type` 字段区分（`user` / `supplier` / `admin`）

为什么：身份业务边界清晰、token 互不干扰，复用现有 token infra（不切换 driver）。

### D2：数据模型 15 张表（13 新 + 2 扩展）

| 表 | 用途 | 关键字段 |
|---|---|---|
| `mall_users`（扩展 user） | C 端会员 | nickname / avatar / status / balance / password_hash |
| `mall_suppliers`（扩展） | 供应商主体 | name / logo / bio / status(active\|disabled) / is_featured / contact_phone / balance / total_sales / commission_rate |
| `mall_supplier_users` | 供应商登录账号 | supplier_id / username / password_hash / status |
| `mall_cards` | 卡牌主数据 | name / image / team / player / serial_no / description |
| `mall_blind_boxes` | 盲盒 SKU | supplier_id(FK, NOT NULL) / name / cover / price / status / on_sale / is_featured |
| `mall_card_pools` | 卡池（1:1 绑定盲盒） | blind_box_id(UK) |
| `mall_card_pool_items` | 池内卡 | pool_id / card_id / rarity(SSR\|SR\|R\|N) / weight / stock |
| `mall_draw_orders` | 抽卡订单 | user_id / blind_box_id / supplier_id / price / status / source(normal\|seckill) |
| `mall_draw_order_items` | 订单明细 | order_id / card_id / rarity / snapshot_name / snapshot_image |
| `mall_promotions` | 限时特价 | supplier_id / blind_box_id / original_price / promo_price / start_at / end_at / status |
| `mall_settlements` | 结算单 | supplier_id / period_start / period_end / total_amount / commission_rate / commission_amount / payout_amount / status / paid_at / paid_by |
| `mall_settlement_items` | 结算明细 | settlement_id / draw_order_id(UK) / amount / commission_amount / payout_amount |
| `mall_platform_ledger` | 平台收入台账（counter 聚合） | counter_key(PK) / amount |
| `mall_seckill_activities` | 秒杀活动主体 | supplier_id / blind_box_id / seckill_price / total_stock / per_user_limit / start_at / end_at / status / redis_initialized |
| `mall_stock_deduction_log` | 秒杀扣减记录 | seckill_id / user_id / blind_box_id / card_id / rarity / snapshot_name / snapshot_image / deducted_at / synced_to_pool_at |
| `mall_user_follow_supplier` | 用户关注供应商关系 | user_id / supplier_id / created_at / deleted_at |

关键约束：
- `mall_blind_boxes.supplier_id` 强制 NOT NULL（纯多供应商模式，平台不自营）
- `mall_card_pool_items.weight` 同池加和固定 10000（避免浮点）
- `mall_promotions` 唯一索引 `(blind_box_id, start_at)`：同一盲盒时间不重叠
- `mall_promotions` 索引 `(status, end_at)`：查"当前生效的活动"
- `mall_settlements` 唯一索引 `(supplier_id, period_start, period_end)`：防重复结算
- `mall_settlement_items.draw_order_id` 唯一索引：一订单只可被结算一次
- `mall_suppliers.commission_rate` 可空：NULL 时使用 config.yaml `default_commission_rate`
- `mall_seckill_activities.total_stock` ≤ mall_card_pool 池内总 stock（创建时校验）
- `mall_stock_deduction_log.synced_to_pool_at` 索引：异步任务扫"待对账"

### D3：抽卡算法事务内并发安全

核心算法（`internal/service/user/draw.go`）：

```
BEGIN TX
  1. SELECT * FROM mall_card_pool_items WHERE pool_id = ? FOR UPDATE
  2. Σ(weight_i) for stock > 0
  3. 若 Σ = 0 → ROLLBACK, ErrSoldOut
  4. rand(0, Σ) → 命中 item
  5. UPDATE mall_card_pool_items
     SET stock = stock - 1
     WHERE id = ? AND stock > 0
     影响行数 = 0 → 重试 1 次（极端并发下别的 tx 已抽走）
  6. SELECT price, COALESCE(promo_price, price) AS actual_price
     FROM mall_blind_boxes bb
     LEFT JOIN mall_promotions p
       ON p.blind_box_id = bb.id
      AND p.status = 'active'
      AND p.start_at <= NOW() AND NOW() < p.end_at
     WHERE bb.id = ?
  7. UPDATE mall_users SET balance = balance - actual_price
     WHERE id = ? AND balance >= actual_price
     影响行数 = 0 → ROLLBACK, ErrInsufficientBalance
  7.5 UPDATE mall_suppliers
      SET balance = balance + actual_price,
          total_sales = total_sales + actual_price
      WHERE id = ?   -- supplier_id
  7.6 INSERT INTO mall_platform_ledger
      (counter_key='total_revenue', amount_delta=actual_price, ...)
      ON DUPLICATE KEY UPDATE amount=amount+VALUES(amount_delta)
      -- 平台台账聚合行，避免每单写多行
  8. INSERT mall_draw_orders (status=paid)
  9. INSERT mall_draw_order_items (snapshot_*)
COMMIT
```

并发安全靠两件事：
- `SELECT ... FOR UPDATE`（MySQL InnoDB 行锁）
- 库存与余额的 `WHERE stock > 0` / `WHERE balance >= ?` 条件更新（避免超卖与超额扣款）

为什么不用乐观锁版本号：库存与余额的「条件更新」语义已经天然原子；乐观锁的 ABA 问题在这里不适用（库存单调递减，不存在回退）。

结算相关字段（7.5 / 7.6）在同一事务内更新，保证：
- 抽卡成功 ↔ supplier.balance 增加 ↔ platform ledger 增加 三者原子
- 抽卡失败 → 全部回滚，supplier 不存在"幻觉收入"

### D4：抽卡活动价用 LEFT JOIN 单查询拿

抽卡路径需要在事务内知道「实际支付价」。两种实现：

- (a) 先查 promotion，再查 blind_box：两次 DB 往返
- (b) LEFT JOIN 一次拿：单次 DB 往返

选 (b)：减少事务内 SQL 次数、LEFT JOIN 明确表达"无活动时原价"的语义。

`mall_promotions` 索引 `(blind_box_id, status, start_at, end_at)` 让 JOIN 高效（实际上 `(blind_box_id, status, end_at)` 复合即可覆盖本查询）。

### D5：缓存策略——多层架构 + 3 个场景，写时失效 + TTL 兜底

**架构**：
```
L1 进程内内存缓存（TTL 5-30s）
  ↓ miss
L2 Redis 缓存（TTL 5min+）
  ↓ miss
L3 MySQL（权威）
```

**接口与实现**：
- `internal/infra/cache/cache.go`：`Cache` 接口（Get / Set / Del / SetNX）
- `internal/infra/cache/memory.go`：L1 内存实现（`github.com/patrickmn/go-cache`，TTL + 容量上限 + 自动过期清扫）
- `internal/infra/cache/redis.go`：L2 Redis 实现（已有，包成 Cache 接口）
- `internal/infra/cache/multi.go`：`MultiLevelCache` 组合：Get 按 L1→L2→DB，Set 反向写 L2+L1，Del 反向失效 L2+L1
- 业务代码只调 `MultiLevelCache` 接口，不感知层级

**缓存场景**：

| Key 模式 | L1 TTL | L2 TTL | 失效方式 | 用途 |
|---|---|---|---|---|
| `blindbox:detail:{id}` | 10s | 10min | 供应商 edit / toggle on_sale 时 DEL L2 + DEL L1（仅本地） | 详情页高读低写 |
| `home:feed:{query_sig}` | 5s | 5min | TTL 自然过期 + 写时 DEL L2 + DEL L1 | 减少 ES 压力 |
| `draw:rate:{uid}:{minute}` | **不走 L1** | 60s | SetNX + EXPIRE 原子 | 抽卡限流必须分布式共享 |

**L1 不走限流的理由**：限流是"全集群共享"的判断，不能让 L1（单进程）失效，否则限流失效。

**写时反向失效**：
```
DB 事务提交
  ↓
MultiLevelCache.Del(key)
  ├─ Redis: DEL key      -- 全集群失效
  └─ Memory: delete key  -- 仅本地进程失效
```

**L1 跨进程最终一致**：L1 在每个进程独立 TTL 过期，最坏 30s 内不同进程会看到不同值。MVP 业务接受（详情页 30s 内可能不一致）。

**库存、用户 balance 不缓存**——事务内必须真值。

**穿透保护**：L1/L2 缓存 `ErrNotFound` 占位（短 TTL 如 30s），防止恶意 key 打 DB。

**为什么不用 LRU 而用 TTL**：MVP 缓存场景固定 3 类，容量可控；TTL 简单可靠。

### D5.1：缓存防穿透——Negative Cache（NotFound 占位）

**问题**：攻击者/用户查询不存在的 key → Redis miss → DB miss → 每次都打 DB。高并发下 DB 被打挂。

**方案**：DB 查询返回 `ErrNotFound` 时，缓存一个空值占位（TTL 较短如 30s）；后续相同 key 请求直接命中空值，返回"不存在"。

**统一抽象**：所有缓存层（L1/L2/Hotspot）共用一套防穿透语义：
- DB 返回 `ErrNotFound` → 缓存 `notFound` 占位 → 后续读直接返回 `ErrNotFound`
- TTL 较短（30s-60s）防止"数据真的不存在但被恶意长期占位"
- 写时主动 invalidate 占位（业务数据写入后失效）

**实现位置**：
- L1/L2 缓存：在 `MultiLevelCache.Get` 的 loader 中检测 `ErrNotFound`，写回 `notFound` marker
- Hotspot 缓存：HotspotLoader 返回 `ErrNotFound` 时缓存 `notFound`（带 `RefreshedAt=now`）
- 业务 service：不必感知，loader 函数本身决定是否返回 `ErrNotFound`

**notFound 数据结构**（L1/L2 用）：
```go
// sentinel：单独的类型 + JSON 标记
type notFoundSentinel struct{}
// 序列化：{"_notFound": true}
// 反序列化：检测到 _notFound 字段 → 返回 ErrNotFound
```

**Hotspot 用**：data 字段是 `any`，可以缓存 `*notFoundPayload`（带 RefreshedAt），staleAfter 后重新触发 loader。

**关键决策**：
- 不用 bloom filter（额外依赖 + 误判）
- 不用 rate limit（已有 L1/L2 限流，覆盖正常用户；穿透防护针对攻击者）
- 不用持久化 negative cache（短 TTL 足够）

为什么详情缓存 + ES 缓存 + 限流三类，其他不做：
- 库存/balance 一致性复杂，超卖风险高，MVP 不承担
- token session：MVP 不切换 driver，仍用 database
- 排行榜/计数器：MVP 没有此类功能

### D6：ES 索引只读 + 离线全量同步

- 索引：`mall_blind_box_index`、`mall_supplier_index`
- 过滤：盲盒 `status=active AND on_sale=true AND is_featured=true`，供应商 `status=active AND is_featured=true`
- 应用进程**只读 ES**，不写 ES
- `cmd/es-sync` 独立入口，全量读 MySQL → 全量覆盖 ES 索引（delete + bulk index），幂等可重跑
- 触发方式：手动（开发态）/ CI pipeline / 部署 hook / cron
- ES 不可用：应用启动期 fail fast；运行期首页接口直接 5xx，不 fallback

为什么独立脚本而非应用内同步：
- 写时同步：应用 hot path 增加延迟
- CDC / binlog：依赖 canal/debezium，复杂度爆炸
- 离线全量脚本：足够 MVP（数据量小、变更频率低），简单可调试

### D7：ES 文档形态是"投影"，不是 MySQL 镜像

ES 文档只存"首页推荐需要的字段"：

```json
// mall_blind_box_index
{
  "id": 1,
  "supplier_id": 10,
  "supplier_name": "Panini 旗舰店",
  "name": "2024 NBA 球星卡盲盒",
  "cover": "https://cdn.example.com/.../cover.jpg",
  "price": 99.00,
  "promo_price": 79.00,
  "rarity_summary": "SSR 3% / SR 12% / R 35% / N 50%",
  "hot_score": 95,
  "created_at": "2026-09-30T10:00:00Z"
}
```

详情字段（卡池 item 列表、活动时间等）从 MySQL 取。这样 ES 索引轻、详情缓存（Redis）承担详情聚合的缓存责任。

### D8：限流——抽卡每用户每分钟 N 次

`internal/middleware/rate_limit.go` 提供 `RateLimitPerMinute(uid int64, n int) gin.HandlerFunc`：
- Key：`draw:rate:{uid}:{minute}`（minute = `time.Now().Unix() / 60`）
- 算法：`SET key 1 NX EX 60`
- 失败 → 429

为什么按分钟而非秒级粒度：盲盒抽卡是"事件型"动作（用户主动点击），秒级粒度过严；分钟级 N 次既能防刷又不影响体验。

MVP 设定 `n = 30`（每用户每分钟 30 次，约每 2 秒 1 次的频率上限）。

### D9：供应商状态与盲盒状态解耦

`mall_suppliers.status` 与 `mall_blind_boxes.on_sale` 是两个独立字段：

- 供应商被禁用 → 该供应商所有盲盒在前端不可见，但**不**自动下架（管理员可单独恢复）
- 盲盒下架（on_sale=false）→ 仅该盲盒不可抽，供应商仍可登后台

service 层在前端列表过滤时做"供应商状态 active + 盲盒状态 active + on_sale + is_featured"四条件 AND。

### D10：同一盲盒同时只 1 个生效活动

`mall_promotions` 唯一索引 `(blind_box_id, start_at)` 防止重叠。运营创建新活动时，service 层校验与现有未结束活动是否冲突；冲突 → 422。

为什么不允许多活动叠加：MVP 阶段简化逻辑、避免叠加规则复杂度。后续要做叠加再升级。

### D11：前端 layout 风格对齐 admin 但独立目录

- `web/src/layouts/user/` 复用 admin 的 `container/`、`router-view/` 组件，但主题色改篮球橙蓝
- `web/src/layouts/supplier/` 同上
- `web/src/router/static/userBase.ts` `supplierBase.ts` 静态子路由注册
- `web/src/lang/{zh-cn,en}/user.yaml` `supplier.yaml` 新建 i18n 文件

为什么不复用 admin layout：视觉与业务不同身份需要不同的导航/菜单。组件级别（`container/`、`router-view/`）可复用。

### D12：错误码统一在 handler 层映射

- service 层返回 sentinel error（`ErrSoldOut` / `ErrInsufficientBalance` / `ErrBlindBoxNotFound` 等）
- handler 层统一映射到 HTTP 状态码 + 业务错误码（参考 `handler/admin/manager.go` 的 `code.go` 模式）
- 前端根据业务错误码决定 UI（余额不足 → 提示充值；已售罄 → 灰色按钮）

### D13：结算流程——按周期聚合 + 手动生成 + mark_paid

**业务语义**：抽卡成功后，钱"先到平台"（体现在 supplier.balance + 平台台账）；admin 周期性触发结算，按周期聚合该供应商所有 paid 订单，扣除手续费后生成结算单；admin 线下打款后 mark_paid。

**结算单生成算法**（`internal/service/admin/settlement.go` `Generate`）：

```
BEGIN TX
  1. SELECT * FROM mall_draw_orders
     WHERE supplier_id = ?
       AND status = 'paid'
       AND created_at >= period_start AND created_at < period_end
       AND id NOT IN (SELECT draw_order_id FROM mall_settlement_items)
     FOR UPDATE
  2. 若 0 行 → ROLLBACK, ErrNoOrdersToSettle
  3. commission_rate := supplier.commission_rate ?? config.DefaultCommissionRate
  4. total := SUM(price)
  5. commission := ROUND(total × commission_rate, 2)
  6. payout := total - commission
  7. INSERT mall_settlements (..., status='pending')
  8. 批量 INSERT mall_settlement_items（每条订单一行）
COMMIT
```

**mark_paid 算法**：

```
BEGIN TX
  1. SELECT * FROM mall_settlements WHERE id = ? AND status = 'pending' FOR UPDATE
  2. UPDATE mall_settlements
     SET status = 'paid', paid_at = NOW(), paid_by = ?, remark = ?
  3. UPDATE mall_suppliers
     SET balance = balance - payout_amount
     WHERE id = ? AND balance >= payout_amount
     影响行数 = 0 → ROLLBACK, ErrInsufficientSupplierBalance
COMMIT
```

**为什么手续费率快照在结算单上**：避免供应商后续调整 commission_rate 影响历史结算单。

**为什么不做真实打款**：MVP 阶段 admin 线下打款（银行转账 / 微信），后台 mark_paid 记账即可。

**为什么 supplier.balance 是"待结算总额"而非"已结算总额"**：
- 抽卡 +price 进入 supplier.balance（待结算）
- mark_paid 时 -payout_amount 出 supplier.balance（已打款）
- balance 字段本质 = "∑ 抽卡收入 - ∑ 已打款"，单一数字表达应付未付

**为什么不自动定时**：MVP 避免依赖 cron 调度器复杂度；admin 手动可控（运营节奏可调）。后续再加 cron。

**为什么不允许多结算单重叠**：`mall_settlements` 唯一索引 `(supplier_id, period_start, period_end)` 防止重复结算。

**preview 接口**：返回该供应商 + 周期范围内的预览数据（订单数 / 总金额 / 预估手续费 / 预估打款额），不写库，供 admin 决策参考。

### D14：平台台账用 counter 聚合行

**为什么用 `mall_platform_ledger` 单行 counter 而不是每笔订单一行**：
- 平台台账只关心"累计收入 / 累计手续费"等少量聚合指标
- 每笔订单写一行会导致表快速膨胀（百万订单 = 百万行），且聚合查询慢
- 单行 counter + `ON DUPLICATE KEY UPDATE` 累加，O(1) 写入、O(1) 读取

**记录哪些指标**：
- `total_revenue` —— 累计抽卡 GMV
- `total_commission` —— 累计平台手续费（结算时 -commission_amount 也累加扣减）

**MVP 不做**：分时序台账（按天 / 按月）、审计追溯（每笔明细）。

### D15：秒杀活动——Redis 预扣 + 异步落库

**业务场景**：秒杀是高频短时间窗活动（如 1000 个名额 1 分钟抢完）。MySQL 单库事务扛不住这种瞬时高并发，需要 Redis 预扣抗压力。

**数据模型新增**：
```
mall_seckill_activities  -- 秒杀活动主体（独立于 mall_promotions）
  id, supplier_id, blind_box_id, 
  seckill_price, total_stock, per_user_limit,
  start_at, end_at, status(active/disabled),
  redis_initialized,     -- Redis 库存是否已初始化（防重复 init）
  created_at, updated_at, deleted_at

mall_stock_deduction_log  -- 库存扣减记录（高频写入，异步同步到卡池库存）
  id, seckill_id, user_id, blind_box_id, card_id,
  rarity, snapshot_name, snapshot_image,
  deducted_at, synced_to_pool_at NULL  -- NULL = 未同步
```

**索引**：
- `mall_stock_deduction_log` 索引 `(synced_to_pool_at, deducted_at)` —— 异步任务查"待同步的扣减"
- `mall_stock_deduction_log` 索引 `(seckill_id)` —— 按秒杀活动聚合扣减

**Redis Key**：
```
seckill:stock:{seckill_id}              -- 整数，秒杀名额，秒杀活动创建时初始化 = total_stock
seckill:user_bought:{sid}:{uid}         -- 用户已购次数，INCR
```

**秒杀下单流程**：
```
1. 预校验：seckill.status='active' AND redis_initialized=true AND NOW() IN [start_at, end_at)
2. 限购校验：
   n = INCR seckill:user_bought:{sid}:{uid}  -- 先 INCR 后比较
   if n > per_user_limit:
       DECR seckill:user_bought:{sid}:{uid}  -- 回滚
       return ErrUserLimitExceeded (HTTP 429)
3. Redis 库存预扣：
   remaining = DECR seckill:stock:{sid}
   if remaining < 0:
       INCR seckill:stock:{sid}              -- 回滚
       DECR seckill:user_bought:{sid}:{uid}  -- 回滚
       return ErrSoldOut (HTTP 409)
4. MySQL 事务（轻量）：
   a. SELECT * FROM mall_card_pool_items WHERE pool_id=? AND stock>0 FOR UPDATE
   b. 抽卡算法（rand + UPDATE stock -= 1 WHERE id=? AND stock>0）
   c. INSERT mall_stock_deduction_log (deducted_at=NOW(), synced_to_pool_at=NULL)
   d. INSERT mall_draw_orders (price=seckill_price, status=paid, source='seckill')
   e. INSERT mall_draw_order_items (snapshot_*)
   f. UPDATE mall_users balance -= seckill_price (条件更新)
   g. UPDATE mall_suppliers balance += seckill_price + platform ledger
5. 失败回滚：
   ROLLBACK
   INCR seckill:stock:{sid}
   DECR seckill:user_bought:{sid}:{uid}
   return 错误
```

**关键决策点**：

1. **秒杀活动创建时立即初始化 Redis 库存**：在事务内 INSERT mall_seckill_activities + SET seckill:stock:{id} total_stock + SET redis_initialized=true，保证原子（避免 Redis 已 init 但 DB 没建行的脏数据）。

2. **秒杀路径下卡池库存立即扣减**：与 D3 普通抽卡一样，秒杀事务内也扣 mall_card_pool_items.stock（保证抽卡正确性）。"异步同步"指 `mall_stock_deduction_log` → 写入对账/审计表，不影响 mall_card_pool_items。

3. **但 Redis 库存与卡池库存的关系**：
   - 秒杀名额（Redis）是营销层面的"流量阀门"
   - 卡池库存（MySQL）是物理层面的"真实库存"
   - **约束**：秒杀名额 ≤ 卡池库存（在秒杀活动创建时校验）
   - 这样 Redis DECR 成功后，卡池库存也必然够用

4. **为什么不用纯 Redis 异步落库**：用户秒杀成功看不到抽中了什么，UX 差。秒杀也要"立即抽卡"，所以 MySQL 事务内仍走 D3 的随机抽卡算法。

5. **`mall_stock_deduction_log` 的存在意义**：
   - **审计追溯**：秒杀名下的所有抽卡明细可查
   - **对账**：与 Redis 库存 + 卡池库存三方对账
   - **数据回放**：如果 Redis 故障，可以从 log 恢复

**异步同步任务**（`cmd/stock-sync`，每分钟跑一次）：
```
1. SELECT * FROM mall_stock_deduction_log
   WHERE synced_to_pool_at IS NULL
   ORDER BY deducted_at LIMIT 1000
2. 聚合：按 blind_box_id 计算每个池的扣减数
3. UPDATE mall_stock_deduction_log SET synced_to_pool_at=NOW() WHERE id IN (...)
   -- 注意：这里不实际扣 mall_card_pool_items（秒杀路径已扣过），只标记同步完成
4. 输出统计：synced_count, synced_pools
```

为什么"异步同步"只是标记而非真扣：秒杀路径下 mall_card_pool_items 已经在事务内扣过（步骤 4.b），stock_deduction_log 只是审计表。这里"同步"是把审计表标记为"已对账完成"，不做实际库存变更。

**如果未来要支持"无抽卡的秒杀资格"**（如秒杀只抢名额不抽卡），则异步同步需要做实际库存扣减——届时扩展。

### D16：进程内 L1 内存缓存的选型与边界

**选型**：使用 `github.com/patrickmn/go-cache`（轻量、TTL 内置、过期自动清扫、API 简洁）。

**配置**：
```yaml
# config/cache.yaml
l1:
  default_ttl: 10s        # 默认 L1 TTL
  cleanup_interval: 60s   # 过期清扫频率
  max_size: 10000         # 最大 key 数（防 OOM）
  
l2:
  blindbox_detail_ttl: 10m
  home_feed_ttl: 5m
  rate_limit_ttl: 60s
```

**L1 容量上限的处理**：go-cache 默认无上限，加容量上限需要 LRU 淘汰。MVP 简化：用 `defaultExpiration` + 主动删除（达到 max_size 时清掉最旧的）。

**为什么不选 LRU 库**：
- LRU 引入额外依赖
- MVP 缓存场景固定 3 类，容量可控
- TTL 过期自然控制大小

**L1 vs L2 数据一致性**：
- L1 TTL ≤ 30s，L2 TTL ≥ 5min
- L1 先过期 → 查 L2 → 回填 L1
- 写时：DB commit → DEL L2（Redis）→ DEL L1（memory）
- 跨进程不一致窗口：L1 在每个进程独立 TTL 过期（5-30s），最坏 30s

**业务接受范围**：
- 盲盒详情 30s 内不一致可接受（用户感知不到）
- 首页 feed 30s 内不一致可接受
- 限流**必须 L2 唯一权威**（不能 L1 缓存）

### D17：MQ 多驱动抽象 + Redis Stream 实现（MVP 仅 Redis）

**业务动机**：
- 秒杀扣减记录的异步对账需要触发机制，cron 轮询延迟高（最坏 1 分钟）
- MQ 解耦发布方与消费方，未来切换 Kafka/RabbitMQ 不动业务代码

**抽象层**：
```
internal/infra/mq/
  ├── mq.go         MQ 接口：Publish(topic, payload) / Subscribe(topic, handler)
  ├── redis.go      Redis Stream 实现（用消费组 + ack）
  └── mock.go       内存实现（单测用，可选）
```

**MVP 仅实现 Redis Stream**（Kafka / RabbitMQ 后续 driver 扩展，不动业务代码）。

**使用场景（MVP）**：
1. 秒杀路径：事务提交后异步 PUBLISH `stock.deduction.sync` 消息，消费者 cmd/stock-sync 处理
2. 未来扩展（不在本期）：活动结束通知、订单创建通知、结算生成通知

**接口签名**：
```go
type MQ interface {
    // Publish 同步发送；返回 err 仅表示消息未到 broker，不代表业务失败
    Publish(ctx context.Context, topic string, payload []byte) error

    // Subscribe 阻塞消费 topic，handler 返回 nil → ack，返回 err → 不 ack（保留 pending 重试）
    Subscribe(ctx context.Context, topic, consumerGroup string, handler func(payload []byte) error) error
}
```

**Redis Stream 实现要点**：
- `XADD topic * payload <bytes>` 发布
- 消费组：`XGROUP CREATE topic group $ MKSTREAM`（首次启动时）
- 消费：`XREADGROUP GROUP group consumer COUNT 1 BLOCK 5000 STREAMS topic >`
- ack：`XACK topic group message_id`
- 失败处理：handler 返回 err → 不 ack → pending list 保留，下次重试（最多 N 次后进 dead letter）

**秒杀路径集成**：

```
抽卡事务 COMMIT
  ↓
go func() {
    if err := mq.Publish(ctx, "stock.deduction.sync", json.Marshal(log)); err != nil {
        log.Warn("publish mq failed, will rely on cron fallback", err)
    }
}()
```

**为什么不把 MQ publish 放在事务内**：
- 事务内同步 publish 增加事务持有时间（影响秒杀高并发性能）
- MQ 失败不应阻塞业务主路径（业务数据已落库，下次 cron 兜底）
- 弱一致 + cron 兜底是行业惯例（秒杀场景的"最终一致"）

**cmd/stock-sync 双模式**：
- 模式 A：消费者模式（推荐）—— `cmd/stock-sync` 是常驻进程，Subscribe `stock.deduction.sync` 实时处理
- 模式 B：cron 兜底 —— 同进程内启 goroutine 每 5 分钟扫 `synced_to_pool_at IS NULL` 兜底
- MVP 同时支持两种模式（flag 控制）

**driver 切换路径**：
- 未来加 Kafka：新建 `internal/infra/mq/kafka.go`，实现 MQ 接口，config.yaml 加 `mq.driver: kafka`
- 业务代码不变（通过 interface 调用）

### D18：第三方渠道对接——适配器 + 工厂模式

**业务动机**：
- MVP 阶段抽卡"模拟支付"、结算"手动 mark_paid"
- 未来对接真实支付（微信 / 支付宝）、真实银行打款 API、短信下发
- 通过适配器接口 + 工厂模式解耦业务代码与具体 driver

**抽象层**：
```
internal/infra/channel/
  ├── channel.go       Channel 接口：Pay / Payout / Notify
  ├── factory.go       ChannelFactory.Create(name string) Channel
  ├── mock.go          Mock 实现（记录调用日志，返回成功）— MVP 默认
  ├── bank.go          银行打款实现（MVP 占位 = Mock）
  └── wechat.go        微信支付实现（未来）
```

**接口定义**：
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

**工厂注册表**：
```go
var drivers = map[string]func() Channel{
    "mock":   NewMockChannel,
    "bank":   NewBankChannel,
    "wechat": NewWechatChannel,  // 未来
    "alipay": NewAlipayChannel,  // 未来
}

func (f *channelFactory) Create(name string) (Channel, error) {
    factory, ok := drivers[name]
    if !ok {
        return nil, ErrUnknownChannel
    }
    return factory(), nil
}
```

**业务调用方式**：
```go
// 抽卡 service（未来真实支付）
factory.Create("payment").Pay(ctx, &PayRequest{
    UserID: uid, Amount: actualPrice, OrderNo: orderNo,
})

// 结算 service（未来真实打款）
factory.Create("bank").Payout(ctx, &PayoutRequest{
    SupplierID: sid, Amount: payoutAmount, SettlementID: sid,
})
```

**配置**：
```yaml
# config/channel.yaml
channel:
  payment: mock        # 未来: wechat / alipay
  bank:    mock        # 未来: real bank API
  sms:     mock        # 未来: aliyun sms / tencent cloud
```

**为什么用工厂而不是直接注入 Channel**：
- 业务 service 可能有多个调用点（抽卡 / 结算 / 短信），需要按 name 选择
- 工厂集中管理 driver 注册表，避免每个 service 重复注入

**MVP 不做的事**：
- ❌ 真实支付（依然模拟支付）
- ❌ 真实银行 API（依然手动 mark_paid）
- ❌ 真实短信（依然固定密码登录）
- ❌ channel 单元测试覆盖（mock 实现是 trivial，不需要测）

### D19：订单状态机——状态模式

**业务对象**：
- `mall_draw_orders.status`：pending → paid → drawn（/ failed）
- `mall_settlements.status`：pending → processing → paid（/ failed）

**为什么用状态模式而不是 switch/case**：
- 状态转换规则集中管理（白名单），不易遗漏非法转换
- 每个状态独立可测（不依赖其他状态）
- 新增状态不动现有代码（开闭原则）

**抽象层**：
```
internal/domain/state/
  ├── state.go             State 接口 + OrderContext 接口
  ├── draw_order_state.go  抽卡订单：PendingState / PaidState / DrawnState / FailedState
  └── settlement_state.go  结算单：PendingState / ProcessingState / PaidState / FailedState
```

**State 接口**：
```go
type State interface {
    Name() string
    // CanTransitionTo 检查目标状态是否合法（白名单）
    CanTransitionTo(target State) bool
    // OnEnter 进入该状态时触发的副作用（hook）
    OnEnter(ctx context.Context, order OrderContext) error
}

type OrderContext interface {
    OrderID() int64
    CurrentState() string
    UpdateState(stateName string) error  // 持久化到 DB
}
```

**状态机示例（抽卡订单）**：
```go
// legalTransitions 定义合法转换
var drawOrderTransitions = map[string][]string{
    "pending": {"paid", "failed"},
    "paid":    {"drawn", "failed"},
    "drawn":   {},  // 终态
    "failed":  {},  // 终态
}

func (s *DrawOrderState) CanTransitionTo(target State) bool {
    allowed, ok := drawOrderTransitions[s.Name()]
    if !ok { return false }
    for _, name := range allowed {
        if name == target.Name() { return true }
    }
    return false
}
```

**业务用法**：
```go
// 抽卡事务成功后
order.SetState(state.NewPaidState())
if err := order.Persist(); err != nil { return err }

// 抽中卡片后（事务内）
if !order.CanTransitionTo(state.NewDrawnState()) {
    return ErrInvalidStateTransition
}
order.SetState(state.NewDrawnState())
order.Persist()
```

**状态机的副作用 hook**：
- 进入 `PaidState` 时：发 MQ 消息 + 触发结算待结算金额更新（已经在事务内的不用重复）
- 进入 `DrawnState` 时：发抽卡成功通知（未来）
- 进入 `FailedState` 时：Redis 库存回滚（如果是秒杀路径）

**为什么把状态机抽到 domain/state 包**：
- 跨多个 service 复用（user.draw / admin.settlement）
- 状态转换规则集中定义，避免分散在 service 各处

**MVP 状态机覆盖范围**：
- ✅ 抽卡订单：pending → paid → drawn，failed 兜底
- ✅ 结算单：pending → processing → paid，failed 兜底
- ✅ 转换合法性校验（白名单）
- ✅ OnEnter hook（MVP 只记日志）
- ❌ 状态转换历史表（审计追溯）— YAGNI
- ❌ 状态超时自动转换（pending → failed 超时）— YAGNI

### D20：下单入口校验——责任链模式

**业务场景**：下单前多个校验环节按顺序执行，任一失败立即终止返回错误。

**校验环节（MVP）**：

| 序号 | Check | 失败错误 |
|---|---|---|
| 1 | UserActiveCheck | 用户被禁用 → ErrUserDisabled |
| 2 | BlindBoxBuyableCheck | 盲盒下架 / 供应商 disabled → ErrBlindBoxNotBuyable |
| 3 | TimeWindowCheck | 活动未开始 / 已结束 / 非活动期 → ErrNotInTimeWindow |
| 4 | UserLimitCheck（秒杀） | 用户已购 ≥ per_user_limit → ErrUserLimitExceeded |
| 5 | BalanceCheck | 余额 < actual_price → ErrInsufficientBalance |

**抽卡 Chain**：
```
DrawChain = UserActive → BlindBoxBuyable → TimeWindow → Balance
SeckillDrawChain = UserActive → BlindBoxBuyable → TimeWindow → UserLimit → Balance
```

**抽象层**：
```
internal/domain/chain/
  ├── validator.go    Validator 接口
  └── chain.go        Chain 编排（顺序执行，任一 err 终止）

internal/service/user/draw_check/
  ├── user_active_check.go
  ├── blindbox_buyable_check.go
  ├── time_window_check.go
  ├── user_limit_check.go
  └── balance_check.go
```

**接口定义**：
```go
type Validator interface {
    Name() string
    Validate(ctx context.Context, input *DrawInput) error
}

type Chain struct {
    validators []Validator
}

func (c *Chain) Validate(ctx context.Context, input *DrawInput) error {
    for _, v := range c.validators {
        if err := v.Validate(ctx, input); err != nil {
            return fmt.Errorf("%s: %w", v.Name(), err)
        }
    }
    return nil
}
```

**业务用法（抽卡 service）**：
```go
func (s *drawService) Draw(ctx context.Context, input *DrawInput) (*DrawResult, error) {
    // 1. 校验链（事务外）
    if err := s.drawChain.Validate(ctx, input); err != nil {
        return nil, err
    }
    
    // 2. 进入事务
    return s.tx.Do(ctx, func(tx *gorm.DB) (*DrawResult, error) {
        // ... D3 抽卡算法 ...
    })
}
```

**为什么把校验放在事务外**：
- 校验失败不应持有 DB 连接（事务开启有性能成本）
- 校验失败的错误是业务错误，不是 DB 错误，事务内无意义
- 事务内只做"必须真值"的扣库存 / 扣余额操作

**Redis 限购放在校验链 vs 事务内**：
- 校验链（推荐）：UserLimitCheck 用 Redis INCR + 比较 pre_user_limit，失败立即终止
- 事务内：失败要回滚事务，浪费 DB 资源
- 选校验链：失败 fast，不进事务

**Chain 与 Service 的关系**：
- Chain 是 Service 的私有字段（service 启动时组装）
- Chain 本身不调 Repository，只调 Service / Cache / MQ
- 每个 Validator 独立可测（注入 mock）

**为什么用责任链而不是一个大 Validate 函数**：
- 每个 Check 独立可测（单测覆盖）
- Check 顺序可配置（不同业务路径挂不同 Chain）
- 新增 Check 不动现有代码（开闭原则）

**未来可加的 Check（不在 MVP）**：
- RiskCheck（黑名单 / IP 频率）
- DeviceCheck（设备指纹）
- GeoCheck（地域限制）

### D21：Redis 热点数据缓存——不过期 + 逻辑时间 + 分布式锁 + 异步更新

**业务场景**：
- 盲盒详情、首页 feed、限时特价活动等"高读低写"数据
- 经典 Cache-Aside（miss → DB → 写 cache）模式下，每次 TTL 到期都会出现"雪崩"（多个请求同时 miss → 全部打 DB）
- 用户要求：热点 key 永驻 Redis，用逻辑时间判断是否需要更新，抢锁后异步刷新，老请求直接返回老数据

**核心算法**：
```
GetHotspot(ctx, key, staleAfter):
  1. Redis.Get(key)  → val
  2. if val == nil (首次冷启动)：
       data = db.Query(key)
       data.RefreshedAt = now
       Redis.Set(key, data, TTL=0)  // 不过期
       return data
  3. data = parse(val)
  4. if now - data.RefreshedAt > staleAfter (逻辑时间到期)：
       if lock.TryAcquire("lock:"+key, TTL=5s):
         go func() {  // 异步更新
           defer lock.Release("lock:"+key)
           newData = db.Query(key)
           newData.RefreshedAt = now
           Redis.Set(key, newData, TTL=0)  // 不过期
         }()
       // 不管抢没抢到锁，都返回老数据
  5. return data  // 老数据返回
```

**抽象层**：
```
internal/infra/cache/hotspot/
  ├── hotspot.go        HotspotCache 接口
  ├── redis_hotspot.go  Redis 实现（含分布式锁）
  └── doc.go            设计文档
```

**接口签名**：
```go
type HotspotLoader func(ctx context.Context, key string) (data any, err error)

type HotspotCache interface {
    Get(ctx context.Context, key string, loader HotspotLoader, staleAfter time.Duration) (data any, err error)
    Invalidate(ctx context.Context, key string) error  // 主动失效
}
```

**实现关键**：
- 数据序列化：JSON + 含 `RefreshedAt` 时间戳
- 分布式锁：`SET lock:{key} {uuid} NX EX 5`（SETNX + TTL）
- 锁释放：用 Lua 脚本保证"只删自己的锁"（防误删）
- 异步更新：`go func()` 不阻塞读路径

**应用映射**：

| Key 模式 | staleAfter | Loader | 用途 |
|---|---|---|---|
| `hotspot:blindbox:{id}` | 30s | 查 blind_box + 当前活动 | 热门盲盒详情 |
| `hotspot:feed:home` | 60s | 走 ES 查首页 feed | 首页 feed |
| `hotspot:promotion:{id}` | 10s | 查 promotion | 限时特价活动 |

**业务用法**：
```go
// 抽卡 service / blindbox service 替换原来的 TTL 缓存调用
data, err := hotspot.Get(ctx, "hotspot:blindbox:1", 
    func(ctx, key) (any, error) {
        return s.repo.GetBlindBoxDetail(ctx, extractID(key))
    },
    30*time.Second,
)
```

**主动失效**：
- 供应商编辑盲盒 → `hotspot.Invalidate(ctx, "hotspot:blindbox:{id}")` → Redis DEL
- 下次读触发冷启动路径 → DB 查 → 写回
- 注意：主动失效后第一次读是同步查 DB（不像异步）

**为什么不完全替代 TTL 缓存**：
- TTL 模式自动清理 → 防 OOM
- Hotspot 永驻 → 需要手动管理容量
- 未来加 LRU 淘汰策略时，两者可以共存（TTL 兜底 + Hotspot 热点）

**冷启动保护**：
- 完全 miss 时同步查 DB（不可避免）
- 高并发下用 singleflight 合并请求（避免雪崩）
- `golang.org/x/sync/singleflight` 提供 Do/DoChan

**穿透防护**（与 D5.1 统一）：
- HotspotLoader 返回 `ErrNotFound` 时缓存 `notFound` 占位（RefreshedAt=now）
- 后续相同 key 请求直接返回 `ErrNotFound`，不查 DB
- 主动 invalidate（如盲盒新建后）：下次读触发冷启动路径走 loader

**与其他缓存的关系**：
```
读路径：
  1. Hotspot.Get(key)        ← 优先查热点
     ↓ miss / 老数据
  2. MultiLevelCache.Get     ← 普通 L1/L2 兜底
     ↓ miss
  3. DB                      ← 兜底

写路径：
  1. DB.Commit
  2. Hotspot.Invalidate(key)  ← 主动失效热点
  3. MultiLevelCache.Del(key) ← 主动失效 L1/L2
```

**MVP 落地范围**：
- ✅ Hotspot 抽象 + Redis 实现 + singleflight 合并
- ✅ 应用于盲盒详情（替换之前的 10min TTL）
- ✅ 应用于首页 feed（替换之前的 5min TTL）
- ❌ 限时特价活动（热度一般，先用 TTL）
- ❌ LRU 淘汰策略（容量可控）
- ❌ Redlock（单 Redis SETNX 足够）

**为什么用 singleflight**：
- 高并发下 Hotspot 完全 miss 时避免 N 个请求同时打 DB
- 第一个请求查 DB，其他请求等待结果复用
- 典型场景：缓存重启后瞬时高并发

### D23：缓存预热 + TTL 随机抖动

**业务动机**：
- 限时特价 / 秒杀活动开始瞬间，会有大量请求涌入
- 如果此时缓存是冷的，每个请求都触发 cold start → DB 单点打挂
- 解决方案：活动创建时**预先**触发相关缓存加载，让活动开始时缓存已是热的

**预热触发时机**：
- 供应商创建 `mall_promotion`（限时特价）成功后
- 供应商创建 `mall_seckill_activity`（秒杀活动）成功后
- 事务 COMMIT 后 `go func()` 异步执行（不阻塞主业务）

**预热对象**：

| 预热对象 | Key 模式 | Loader | staleAfter |
|---|---|---|---|
| 该供应商的所有盲盒详情 | `hotspot:blindbox:{id}` | 查 blind_box + 当前活动 | 30s |
| 关注该供应商的用户的首页 feed | `hotspot:feed:user:{uid}` | 走 ES 查该用户推荐 feed | 60s |

**预热算法**：
```
OnPromotionCreated(promotion *MallPromotion):
  go preheat.PromotionPreheat(promotion)

func PromotionPreheat(p *MallPromotion):
  // 1. 预热该供应商的所有盲盒详情
  blindBoxIDs = repo.ListActiveBlindBoxIDs(p.SupplierID)
  for id in blindBoxIDs:
    hotspot.Get(ctx, "hotspot:blindbox:"+id, blindBoxLoader, 30*time.Second)
  
  // 2. 预热关注该供应商的用户的 feed
  followerIDs = repo.ListFollowerIDs(p.SupplierID)
  for uid in followerIDs:
    hotspot.Get(ctx, "hotspot:feed:user:"+uid, feedLoader, 60*time.Second)
```

**新增实体**：`mall_user_follow_supplier`（用户关注供应商关系表）

| 字段 | 类型 | 默认值 | 可空 | 说明 |
|---|---|---|---|---|
| `id` | bigint | auto | 否 | 主键 |
| `user_id` | bigint | 0 | 否 | FK mall_users.id |
| `supplier_id` | bigint | 0 | 否 | FK mall_suppliers.id |
| `created_at` | datetime(3) | now(3) | 否 | GORM 自动 |
| `deleted_at` | datetime(3) | NULL | 是 | 软删除 |

唯一索引：`(user_id, supplier_id)`

**用户关注 / 取关接口**（MVP）：
```
POST   /api/v1/user/follow/suppliers/:supplier_id   关注
DELETE /api/v1/user/follow/suppliers/:supplier_id   取关
GET    /api/v1/user/follow/suppliers                我的关注列表
```

**TTL 随机抖动**：

```
baseTTL = 30s (Hotspot) / 10min (Redis L2) / 5min (Feed L2)
actualTTL = baseTTL + rand[0, baseTTL/4)  // +0%~+25% 正抖动
```

**为什么正抖动而非 ±20% 对称**：
- 正抖动让 TTL 只增不减，避免"提前过期"导致数据不一致窗口
- 防雪崩目标：错开过期时间，对称还是正抖动都能达到，正抖动更保守

**实现位置**：
- `internal/infra/cache/jitter.go`：封装 `JitterTTL(base time.Duration) time.Duration`
- 所有缓存 TTL 配置经过此函数包装

**预热失败处理**：
- 预热异步 goroutine 失败仅记 warn 日志
- 不影响主业务（活动创建已成功）
- 下次正常请求触发 cold start 路径补齐

**预热的边界（不做）**：
- ❌ 不预热所有用户的 feed（成本不可控）
- ❌ 不预热秒杀活动本身（秒杀名额是 Redis 计数，不是 cache）
- ❌ 不预热历史订单（量太大）
- ✅ 只预热"活动关联的供应商"范围

**为什么用 goroutine 而非 MQ 消息**：
- 预热是 fire-and-forget，不需要持久化
- 不需要重试（失败靠下次请求补齐）
- goroutine 启动成本比 MQ publish 低（无序列化、无 broker IO）
- MVP 简化：先 goroutine，量大了再迁 MQ

## Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        Frontend (Vue 3)                       │
│  /user/*  /supplier/*  /admin/* (含结算单管理)                │
└──────────────────────────────────────────────────────────────┘
                          │ HTTP/JSON
                          ▼
┌──────────────────────────────────────────────────────────────┐
│                       Gin Router                              │
│   /api/v1/user/*  /api/v1/supplier/*  /api/v1/admin/*         │
│   + 统一中间件：Auth / RateLimit / CORS                        │
└──────────────────────────────────────────────────────────────┘
                          │
        ┌─────────────────┼─────────────────┐
        ▼                 ▼                 ▼
   ┌─────────┐      ┌──────────┐      ┌────────────┐
   │  user/  │      │ supplier/│      │   admin/   │
   │  handler│      │  handler │      │   handler  │
   └────┬────┘      └─────┬────┘      └─────┬──────┘
        │                 │                 │ (含 settlement)
        ▼                 ▼                 ▼
   ┌─────────┐      ┌──────────┐      ┌────────────┐
   │  user/  │      │ supplier/│      │   admin/   │
   │ service │      │  service │      │   service  │
   └────┬────┘      └─────┬────┘      └─────┬──────┘
        │                 │                 │
        ▼                 ▼                 ▼
   ┌─────────────────────────────────────────────────┐
   │           Repository Layer (GORM)               │
   │  user/  supplier/  admin/  + base CRUDService   │
   └─────────────────────┬───────────────────────────┘
                         │
        ┌────────────────┼────────────────┐
        ▼                ▼                ▼
   ┌─────────┐      ┌──────────┐     ┌──────────┐
   │  MySQL  │      │  Redis   │     │ ES (read │
   │ (write  │      │ (cache + │     │  only)   │
   │ + read) │      │ rate     │     │          │
   │         │      │ limit)   │     │          │
   └─────────┘      └──────────┘     └──────────┘
                         ▲
                         │ bulk index (offline)
                         │
                  ┌──────────────┐
                  │  cmd/es-sync │
                  │ (MySQL→ES)   │
                  └──────────────┘

资金流（事务内）:
  抽卡 → user.balance -= price
       → supplier.balance += price (待结算)
       → platform_ledger.total_revenue += price
  结算（admin 手动）:
       → 生成 settlement 单（pending）→ mark_paid
       → supplier.balance -= payout_amount
       → platform_ledger.total_commission += commission_amount
```

## Cross-cutting Concerns

### 配置加载

新增两个配置文件：

`config/cache.yaml`：
```yaml
redis:
  host: 127.0.0.1
  port: 6379
  password: ""
  db: 0
  pool_size: 50
```

`config/search.yaml`：
```yaml
elasticsearch:
  addresses:
    - http://127.0.0.1:9200
  username: ""
  password: ""
  index_prefix: mall
```

`config/.env.yaml.example` 同步追加字段。

### 启动期

`cmd/serve/main.go` 在 `database.Init()` 之后：
- `cache.Init()` —— 连接 Redis，失败 log.Fatal
- `search.Init()` —— 连接 ES，失败 log.Fatal

新增 migrations 000006-000010 在已有 `runMigrations` 链路里自动跑通。

### 测试策略

- **单测**：service 层 mock repository；handler 层 stub service（沿用 admin-management 范式）
- **集成测试**：抽卡并发安全测试用真实 MySQL（sqlmock 不够，要 tx 行为）
- **E2E**：本期不做，留后续 change

### 可观测性

- 抽卡接口加 structured log（uid / blind_box_id / card_id / price / latency）
- ES 同步脚本输出统计（同步数量、用时）
- 缓存命中率：v0 不做（基础设施还没），留埋点

## Open Questions

无。当前 change 内所有关键决策（D1-D12）已经用户确认。

## Reference

- 借鉴产品：泡泡玛特（POPMART）、卡游（Kayou）、52TOYS（概率公示 + 随机抽取的实体卡盲盒）
- 借鉴结算模式：Shopify Payments / 微信支付商户平台（按周期结算 + 平台手续费）
- ES Go client：github.com/elastic/go-elasticsearch/v8
- Redis Go client：github.com/redis/go-redis/v9
- 现有 token infra：internal/infra/token/（database driver，type 字段区分身份）
