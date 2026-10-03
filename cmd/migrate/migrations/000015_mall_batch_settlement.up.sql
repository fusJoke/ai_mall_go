-- +migrate Up
--
-- 批次结算（openspec: add-batch-settlement）——商品即批次，1 商品 = 1 账单。
--
-- 资金流回顾（沿用设计 D13/D14，不变）：
--   - 抽卡成功 → supplier.balance += actual_price（待结算）
--   - 售罄（draw 事务内自动）→ 生成账单 generated（聚合该商品全部 drawn 订单快照）
--   - 供应商确认 → confirmed；admin MarkPaid → supplier.balance -= payout_amount
--   - 账单 paid 与商品 lifecycle=settled 同事务落库
--
-- 索引说明：
--   - mall_settlement_bills (blind_box_id) UK：一商品一账单（1:1），重复生成在
--     DB 层就拒绝（幂等兜底双保险：service 层先查，DB 层 UK 兜底）。
--   - mall_settlement_bills (supplier_id, status, created_at)：供应商账单中心按状态 tab。
--   - mall_settlement_bills (status, created_at)：admin 后台筛待打款/待重核。
--   - mall_settlement_bill_items (draw_order_id) UK：一订单只可被结算一次
--     （与周期结算 mall_settlement_items 同规则；两表并存、按商品/按周期互斥使用）。
--   - mall_blind_boxes (supplier_id, lifecycle)：供应商按生命周期筛商品
--     （销售中/售罄结算中/已结算）。

-- ---------------------------------------------------------------------------
-- 扩展 mall_blind_boxes：加 lifecycle（商品即批次的生命周期）
--   selling=销售中 / sold_out=售罄结算中 / settled=已结算归档。
--   与 status(启停) / on_sale(上下架) 正交：下架可逆不触发结算；售罄自动进入结算流。
-- ---------------------------------------------------------------------------
ALTER TABLE `mall_blind_boxes`
  ADD COLUMN `lifecycle` varchar(16) NOT NULL DEFAULT 'selling'
    COMMENT '生命周期(selling=销售中,sold_out=售罄结算中,settled=已结算归档)' AFTER `on_sale`,
  ADD INDEX `idx_mall_blind_boxes_supplier_lifecycle` (`supplier_id`, `lifecycle`);

-- ---------------------------------------------------------------------------
-- mall_settlement_bills：批次账单主表（1 商品 = 1 账单）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `mall_settlement_bills` (
  `id`                bigint        NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `blind_box_id`      bigint        NOT NULL                                            COMMENT '商品ID(1商品1账单)',
  `supplier_id`       bigint        NOT NULL                                            COMMENT '供应商ID(冗余,免join)',
  `total_amount`      decimal(14,2) NOT NULL    DEFAULT 0                              COMMENT '本批总GMV(元)',
  `commission_rate`   decimal(5,4)  NOT NULL                                            COMMENT '本单手续费率(快照)',
  `commission_amount` decimal(14,2) NOT NULL    DEFAULT 0                              COMMENT '手续费(元)',
  `payout_amount`     decimal(14,2) NOT NULL    DEFAULT 0                              COMMENT '应打款额(元)',
  `status`            varchar(16)   NOT NULL    DEFAULT 'generated'                     COMMENT '状态(generated/confirmed/rejected/failed/paid)',
  `reject_reason`     varchar(500)  NOT NULL    DEFAULT ''                              COMMENT '驳回原因(供应商驳回时必填)',
  `version`           int           NOT NULL    DEFAULT 1                               COMMENT '重核版本号(rejected重核后+1)',
  `confirmed_at`      datetime(3)   DEFAULT NULL                                        COMMENT '供应商确认时间',
  `paid_at`           datetime(3)   DEFAULT NULL                                        COMMENT '打款时间',
  `paid_by`           bigint        DEFAULT NULL                                        COMMENT '打款人(admin.id)',
  `remark`            varchar(500)  DEFAULT NULL                                        COMMENT '备注(admin重核说明等)',
  `updated_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                                  ON UPDATE CURRENT_TIMESTAMP(3)        COMMENT '更新时间',
  `created_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)          COMMENT '创建时间',
  `deleted_at`        datetime(3)   DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_settlement_bills_blind_box` (`blind_box_id`),
  KEY        `idx_mall_settlement_bills_supplier_status` (`supplier_id`, `status`, `created_at`),
  KEY        `idx_mall_settlement_bills_status_created`  (`status`, `created_at`),
  KEY        `idx_mall_settlement_bills_deleted_at`      (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- mall_settlement_bill_items：批次账单明细（一订单一行，金额快照）
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `mall_settlement_bill_items` (
  `id`                bigint        NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `bill_id`           bigint        NOT NULL                                            COMMENT '账单ID',
  `draw_order_id`     bigint        NOT NULL                                            COMMENT '抽卡订单ID',
  `amount`            decimal(10,2) NOT NULL DEFAULT 0                                  COMMENT '订单金额(元)',
  `commission_amount` decimal(10,2) NOT NULL DEFAULT 0                                  COMMENT '手续费(元)',
  `payout_amount`     decimal(10,2) NOT NULL DEFAULT 0                                  COMMENT '应打款额(元)',
  `updated_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                                  ON UPDATE CURRENT_TIMESTAMP(3)        COMMENT '更新时间',
  `created_at`        datetime(3)   NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)          COMMENT '创建时间',
  `deleted_at`        datetime(3)   DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_settlement_bill_items_draw_order` (`draw_order_id`),
  KEY        `idx_mall_settlement_bill_items_bill`        (`bill_id`),
  KEY        `idx_mall_settlement_bill_items_deleted_at`  (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
