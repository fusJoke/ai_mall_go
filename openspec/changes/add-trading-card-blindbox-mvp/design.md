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
