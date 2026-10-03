-- +migrate Up
--
-- mall_promotions：限时特价活动（同一盲盒同 start_at 不允许重复 UK，
-- 真正的「同盲盒时间段重叠」防重由 service 层在 Create 时校验，
-- 见 internal/service/supplier/promotion.go）。
--
-- 索引说明：
--   - idx_mall_promotions_bb_start UK (blind_box_id, start_at)：
--     防止「同盲盒 + 同起始时间」重复插入。
--   - idx_mall_promotions_status_end (status, end_at)：
--     抽卡事务内 LEFT JOIN 查"当前生效"活动（设计 D4）。
--
-- 价格字段：
--   - original_price / promo_price 都用 decimal(10,2)，
--     promo_price < original_price 由 service 层校验。

CREATE TABLE IF NOT EXISTS `mall_promotions` (
  `id`             bigint        NOT NULL AUTO_INCREMENT                          COMMENT 'ID',
  `supplier_id`    bigint        NOT NULL                                          COMMENT '所属供应商ID',
  `blind_box_id`   bigint        NOT NULL                                          COMMENT '盲盒ID',
  `original_price` decimal(10,2) NOT NULL DEFAULT 0                                COMMENT '原价(元)',
  `promo_price`    decimal(10,2) NOT NULL DEFAULT 0                                COMMENT '活动价(元)',
  `start_at`       datetime(3)   NOT NULL                                          COMMENT '活动开始时间',
  `end_at`         datetime(3)   NOT NULL                                          COMMENT '活动结束时间',
  `status`         varchar(16)   NOT NULL DEFAULT 'active'                         COMMENT '状态(active=启用,disabled=禁用)',
  `updated_at`     datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                            ON UPDATE CURRENT_TIMESTAMP(3)          COMMENT '更新时间',
  `created_at`     datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)            COMMENT '创建时间',
  `deleted_at`     datetime(3)   DEFAULT NULL                                      COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_promotions_bb_start`  (`blind_box_id`, `start_at`),
  KEY        `idx_mall_promotions_status_end` (`status`, `end_at`),
  KEY        `idx_mall_promotions_supplier`  (`supplier_id`),
  KEY        `idx_mall_promotions_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
