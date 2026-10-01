# Proposal: 球星卡盲盒商城 MVP

## Why

`ai-go-mall` 仓库目前已落地完整的后台（admin）端——账号、权限、菜单、上传、初始化、登录流水——但**没有任何 C 端业务**。`model/user.go` 是一个测试占位模型（仅演示 model 自注册），前端 `views/admin/` 是唯一存在的前端业务区，`internal/handler/user/`、`internal/service/user/`、`internal/repository/user/` 三个目录尚未建立。

本 change 落地**球星卡盲盒商城 MVP**，在「概率公示 + 随机抽取 + 多供应商入驻 + 限时特价」这一最小但端到端可演示的商业模型下，把 C 端（用户）、B 端（供应商）、管理端（admin）的核心链路打通，并引入 Redis 缓存层与 Elasticsearch 推荐层作为后续规模化的基础设施。

## What Changes

### 数据模型

- 扩展现有 `model.User` 为 `mall_users`（加 `nickname` / `avatar` / `status` / `balance` / `password_hash`）
- 扩展 `mall_suppliers` 加结算字段（`balance` / `total_sales` / `commission_rate`）
- 新增 13 张表（详见 `design.md` D2）：
  - `mall_suppliers`（含 `balance` / `total_sales` / `commission_rate` 结算字段）
  - `mall_supplier_users` 供应商登录账号
  - `mall_cards` 卡牌主数据
  - `mall_blind_boxes` 盲盒 SKU（FK supplier_id，强制非空）
  - `mall_card_pools` 卡池（1:1 绑定盲盒）
  - `mall_card_pool_items` 池内卡（含 weight / stock / rarity）
  - `mall_draw_orders` 抽卡订单（source='normal'|'seckill'）
  - `mall_draw_order_items` 订单明细
  - `mall_promotions` 限时特价活动（FK blind_box_id + 时间窗）
  - `mall_settlements` 结算单（按周期聚合供应商收入）
  - `mall_settlement_items` 结算明细（关联到 draw_order）
  - `mall_platform_ledger` 平台收入台账（聚合 counter）
  - `mall_seckill_activities` 秒杀活动主体
  - `mall_stock_deduction_log` 秒杀库存扣减记录（高频写入，异步对账）

### 后端代码

- 新增 `internal/infra/cache/` Redis 客户端封装（`Cache` 接口 + Redis 实现）
- 新增 `internal/infra/search/` ES 客户端封装（`SearchClient` 接口 + ES 实现）
- 新增 `internal/handler/user/` `internal/service/user/` `internal/repository/user/`
- 新增 `internal/handler/supplier/` `internal/service/supplier/` `internal/repository/supplier/`
- 新增 `internal/handler/admin/settlement.go` `internal/service/admin/settlement.go` 结算单管理
- 扩展 `internal/handler/admin/` `internal/service/admin/` `internal/repository/admin/` 增加 supplier / blindbox / promotion / settlement 管理能力
- 新增 `internal/middleware/` `rate_limit.go` 抽卡接口限流
- 复用 `internal/infra/token/`（不切换 driver）

### cmd 入口

- 新增 `cmd/es-sync/`：全量把 MySQL「推荐 + 在售」数据同步到 ES
- 扩展 `cmd/seed/`：补充供应商 / 卡牌 / 盲盒 / 活动 / 秒杀种子数据
- 新增 `cmd/stock-sync/`：秒杀扣减记录异步对账脚本（消费者模式 + cron 兜底）
- 扩展 `cmd/migrate/migrations/`：新增 7 个迁移文件（000006-000012）

### 前端

- 新增 `layouts/user/` `layouts/supplier/`（参考 `layouts/admin/` 结构）
- 新增 `views/user/`：登录、首页 feed、盲盒详情、订单列表、订单详情（5 页）
- 新增 `views/supplier/`：登录、商品列表、商品编辑（3 页）
- 新增 `api/user/` `api/supplier/`
- 扩展 `views/admin/`：supplier / blindbox / promotion 管理页
- 新增 `router/static/userBase.ts` `router/static/supplierBase.ts`
- 扩展 `lang/{zh-cn,en}/{user,supplier}.yaml`

### 配置

- 新增 `config/cache.yaml`（Redis 连接信息）
- 新增 `config/search.yaml`（ES 连接信息 + 索引前缀）

### 不改

- 现有 admin 模块（init/manager/rule/group/log/upload/auth/captcha）逻辑保持不变
- `internal/infra/token/` 不切换 driver（仍用 database）
- 现有 5 个 migrations 文件不动

## Capabilities

### New Capabilities

- `trading-card-blindbox-mvp`：球星卡盲盒商城 MVP，14 张表、3 套接口、ES 推荐、两层缓存、活动价、秒杀、限流、结算。
- `mall-cache-multi-level`：两层缓存架构（L1 进程内 5-30s + L2 Redis 5min+ + L3 MySQL），自动回填与反向失效。
- `mall-search-es`：推荐层（ES），离线全量脚本同步，应用只读。
- `mall-rate-limit`：抽卡接口限流（Redis SetNX + TTL，不走 L1）。
- `mall-settlement`：结算流程（按周期聚合 + 手动生成 + admin 标记打款 + 平台手续费）。
- `mall-seckill`：秒杀活动（Redis 预扣 + 异步对账）。
- `mall-mq-multi-driver`：MQ 多驱动抽象层（MVP 仅 Redis Stream 落地）。
- `es-sync-script`：`cmd/es-sync` 全量同步入口。
- `stock-sync-script`：`cmd/stock-sync` 秒杀扣减记录异步对账任务（消费者模式 + cron 兜底）。

### Modified Capabilities

无现有 spec 需要修改——MVP 全部为新增。

## Impact

- **后端代码**：约 50 个新文件、3 个目录族（user/supplier/admin-extension），总代码量估算 5500-6500 行
- **数据库**：新增 13 张表 + 扩展 2 张表（users + suppliers）+ 7 个迁移文件（000006-000012）+ seed 数据扩展
- **依赖**：新增 `github.com/redis/go-redis/v9`、`github.com/elastic/go-elasticsearch/v8`、`github.com/patrickmn/go-cache/v2`
- **基础设施**：本地环境需提供 Redis 与 Elasticsearch（应用启动期连接，fail fast）
- **测试**：每个新 service 配单测；抽卡接口事务并发安全要做集成测试（用真实 MySQL）；秒杀并发测试用真实 Redis + MySQL
- **前端**：约 11 个新页面 + 2 个 layout + 多个 i18n key
- **权限**：admin 后台新增 supplier / blindbox / promotion / seckill / settlement 五组管理规则，需在 migration 000013 落地 admin_rule 种子

## Out of Scope（非目标）

明确不在本期落地：

- 用户注册流程 / 验证码 / 找回密码（用 seed 用户登录即可）
- 真实支付 / 退款 / 抽卡回滚（前端一键即扣 + 后端直接 paid）
- 收藏 / 晒卡 / 集卡册
- 卡牌二级交易 / 求购市场
- 抽卡保底 / 累计次数兑奖
- 供应商资质审核流（admin 可禁用即可）
- ES 实时同步 / CDC / binlog
- token driver 切换到 Redis
- 库存计数器缓存 / 用户余额缓存
- 多 ES 集群 / 多 Redis 哨兵 / 集群模式
- 满减 / 赠品 / 会员折扣等多活动类型
- 活动叠加 / 互斥规则

这些列入后续 change。

### 结算相关非目标（明确不做）

- 自动定时生成结算单（cron / 调度器）
- 真实银行 API 对接（手动 mark_paid 即可，流水记账）
- 结算单的撤销 / 重做 / 驳回
- 退款冲销结算单（与"无退款"YAGNI 联动）
- 多级分销分润
- 平台手续费单独账户（用聚合统计代替）
- 结算单的导出 PDF / Excel（仅 Web 列表）
