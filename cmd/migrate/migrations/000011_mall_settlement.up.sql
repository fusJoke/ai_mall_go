-- +migrate Up
--
-- 结算流程相关表 + mall_suppliers 结算字段扩展。
--
-- 资金流回顾（设计 D13 / D14）：
--   - 抽卡成功 → supplier.balance += actual_price（待结算）
--   - admin Generate 结算单 → 按周期聚合 paid 订单 → 算 commission + payout
--   - admin MarkPaid（线下打款） → supplier.balance -= payout_amount
--
-- 索引说明：
--   - mall_settlements (supplier_id, period_start, period_end) UK：
--     防「同一供应商 + 同一周期」重复结算。
--   - mall_settlements (status, created_at)：admin 后台筛 pending/processing 单。
--   - mall_settlement_items (draw_order_id) UK：一订单只可被结算一次，
--     防重复进入结算单。
--   - mall_platform_ledger (counter_key) PK：counter 行主键。

-- ---------------------------------------------------------------------------
-- 扩展 mall_suppliers：加 balance / total_sales / commission_rate
-- ---------------------------------------------------------------------------
ALTER TABLE `mall_suppliers`
  ADD COLUMN `balance`         decimal(12,2) NOT NULL DEFAULT 0     COMMENT '待结算余额(元)'  AFTER `contact_phone`,
  ADD COLUMN `total_sales`     decimal(14,2) NOT NULL DEFAULT 0     COMMENT '累计销售额(元)'  AFTER `balance`,
  ADD COLUMN `commission_rate` decimal(5,4)  DEFAULT NULL            COMMENT '手续费率(NULL=走config默认)' AFTER `total_sales`;

-- ---------------------------------------------------------------------------
-- mall_settlements：结算单主表
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `mall_settlements` (
  `id`                bigint        NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `supplier_id`       bigint        NOT NULL                                            COMMENT '供应商ID',
  `period_start`      datetime(3)   NOT NULL                                            COMMENT '结算周期起始',
  `period_end`        datetime(3)   NOT NULL                                            COMMENT '结算周期结束',
  `total_amount`      decimal(14,2) NOT NULL    DEFAULT 0                              COMMENT '周期总GMV(元)',
  `commission_rate`   decimal(5,4)  NOT NULL                                            COMMENT '本单手续费率(快照)',
  `commission_amount` decimal(14,2) NOT NULL    DEFAULT 0                              COMMENT '手续费(元)',
  `payout_amount`     decimal(14,2) NOT NULL    DEFAULT 0                              COMMENT '应打款额(元)',
  `status`            varchar(16)   NOT NULL    DEFAULT 'pending'                       COMMENT '状态(pending/processing/paid/failed)',
  `paid_at`           datetime(3)   DEFAULT NULL                                        COMMENT '打款时间',
  `paid_by`           bigint        DEFAULT NULL                                        COMMENT '打款人(admin.id)',
  `remark`            varchar(500)  DEFAULT NULL                                        COMMENT '备注',
  `updated_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                                  ON UPDATE CURRENT_TIMESTAMP(3)        COMMENT '更新时间',
  `created_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)          COMMENT '创建时间',
  `deleted_at`        datetime(3)   DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_settlements_supplier_period` (`supplier_id`, `period_start`, `period_end`),
  KEY        `idx_mall_settlements_status_created`  (`status`, `created_at`),
  KEY        `idx_mall_settlements_deleted_at`     (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- mall_settlement_items：结算明细
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `mall_settlement_items` (
  `id`                bigint        NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `settlement_id`     bigint        NOT NULL                                            COMMENT '结算单ID',
  `draw_order_id`     bigint        NOT NULL                                            COMMENT '抽卡订单ID',
  `amount`            decimal(10,2) NOT NULL DEFAULT 0                                  COMMENT '订单金额(元)',
  `commission_amount` decimal(10,2) NOT NULL DEFAULT 0                                  COMMENT '手续费(元)',
  `payout_amount`     decimal(10,2) NOT NULL DEFAULT 0                                  COMMENT '应打款额(元)',
  `updated_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                                  ON UPDATE CURRENT_TIMESTAMP(3)        COMMENT '更新时间',
  `created_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)          COMMENT '创建时间',
  `deleted_at`        datetime(3)   DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_settlement_items_draw_order` (`draw_order_id`),
  KEY        `idx_mall_settlement_items_settlement`  (`settlement_id`),
  KEY        `idx_mall_settlement_items_deleted_at`  (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- mall_platform_ledger：平台收入台账（counter 聚合行）
-- counter_key 唯一主键，amount 用 ON DUPLICATE KEY UPDATE 累加。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `mall_platform_ledger` (
  `counter_key` varchar(64)  NOT NULL                                                COMMENT '计数器键(如total_revenue)',
  `amount`      decimal(20,2) NOT NULL DEFAULT 0                                     COMMENT '聚合金额',
  `updated_at`  datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                       ON UPDATE CURRENT_TIMESTAMP(3)                COMMENT '更新时间',
  `created_at`  datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)                  COMMENT '创建时间',
  PRIMARY KEY (`counter_key`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- mall_platform_ledger seed：初始化两个核心 counter。
--   - total_revenue：累计抽卡 GMV（每次抽卡 +actual_price）。
--   - total_commission：累计平台手续费（每次 mark_paid -commission_amount）。
-- 用 ON DUPLICATE KEY UPDATE 让迁移可重入。
-- ---------------------------------------------------------------------------
INSERT INTO `mall_platform_ledger` (`counter_key`, `amount`, `updated_at`, `created_at`)
VALUES
  ('total_revenue',   0.00, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)),
  ('total_commission', 0.00, CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE `counter_key` = VALUES(`counter_key`);
