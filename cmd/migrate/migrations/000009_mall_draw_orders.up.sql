-- +migrate Up
--
-- 抽卡订单主表 + 明细表。
--   - mall_draw_orders：单次抽卡即一单（即使一次抽中多张，单 order 包多 items）
--     status 枚举由 internal/domain/state 状态机约束（见 Phase 14）：
--       pending → paid → drawn，failed 兜底。
--   - mall_draw_order_items：抽中的卡牌明细，携带 snapshot_* 字段
--     防止后续卡牌改名 / 改图影响历史订单展示。
--
-- 索引说明：
--   - idx_mall_draw_orders_user_created (user_id, created_at)：用户订单列表分页。
--   - idx_mall_draw_orders_order_no UK：order_no 全局唯一（支付回调对账 / 幂等）。
--   - idx_mall_draw_orders_supplier_status_created (supplier_id, status, created_at)：
--     结算单生成时按 (supplier_id, status='paid', period) 选订单（D13 算法）。
--   - idx_mall_draw_order_items_order (order_id)：查订单明细。
--
-- price 用 decimal(10,2)：单笔抽卡金额上限 99,999,999.99 元，远超业务范围。
-- supplier_id 冗余到 orders：避免 join blind_box 推断（结算 / 列表场景）。

CREATE TABLE IF NOT EXISTS `mall_draw_orders` (
  `id`           bigint        NOT NULL AUTO_INCREMENT                           COMMENT 'ID',
  `order_no`     varchar(32)   NOT NULL                                           COMMENT '订单号(业务唯一)',
  `user_id`      bigint        NOT NULL                                           COMMENT '用户ID',
  `blind_box_id` bigint        NOT NULL                                           COMMENT '盲盒ID',
  `supplier_id`  bigint        NOT NULL                                           COMMENT '供应商ID(冗余,避免JOIN)',
  `price`        decimal(10,2) NOT NULL DEFAULT 0                                 COMMENT '实付金额(元)',
  `status`       varchar(16)   NOT NULL DEFAULT 'pending'                         COMMENT '状态(pending/paid/drawn/failed)',
  `source`       varchar(16)   NOT NULL DEFAULT 'normal'                          COMMENT '来源(normal=普通,seckill=秒杀)',
  `updated_at`   datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                          ON UPDATE CURRENT_TIMESTAMP(3)           COMMENT '更新时间',
  `created_at`   datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)             COMMENT '创建时间',
  `deleted_at`   datetime(3)   DEFAULT NULL                                       COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_draw_orders_order_no`             (`order_no`),
  KEY        `idx_mall_draw_orders_user_created`         (`user_id`, `created_at`),
  KEY        `idx_mall_draw_orders_supplier_status_created` (`supplier_id`, `status`, `created_at`),
  KEY        `idx_mall_draw_orders_blind_box`            (`blind_box_id`),
  KEY        `idx_mall_draw_orders_deleted_at`           (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mall_draw_order_items` (
  `id`             bigint       NOT NULL AUTO_INCREMENT                           COMMENT 'ID',
  `order_id`       bigint       NOT NULL                                           COMMENT '所属订单ID',
  `card_id`        bigint       NOT NULL                                           COMMENT '卡牌ID',
  `rarity`         varchar(8)   NOT NULL                                           COMMENT '稀有度(SSR/SR/R/N)',
  `snapshot_name`  varchar(100) NOT NULL                                           COMMENT '卡名(快照)',
  `snapshot_image` varchar(255) DEFAULT NULL                                       COMMENT '卡图(快照)',
  `updated_at`     datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                          ON UPDATE CURRENT_TIMESTAMP(3)           COMMENT '更新时间',
  `created_at`     datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)             COMMENT '创建时间',
  `deleted_at`     datetime(3)  DEFAULT NULL                                       COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_draw_order_items_order`     (`order_id`),
  KEY `idx_mall_draw_order_items_card`      (`card_id`),
  KEY `idx_mall_draw_order_items_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
