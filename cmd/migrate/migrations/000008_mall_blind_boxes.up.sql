-- +migrate Up
--
-- 盲盒 / 卡牌 / 卡池 4 张表：
--   - mall_cards：卡牌主数据（不可变元数据，库存与稀有度在 pool_items 里）
--   - mall_blind_boxes：盲盒 SKU，supplier_id NOT NULL（纯多供应商模式）
--   - mall_card_pools：1:1 与 blind_box 绑定
--   - mall_card_pool_items：池内卡（rarity / weight / stock）
--
-- 索引说明：
--   - (supplier_id, status, on_sale)：供应商视角筛「在售」盲盒
--   - (is_featured, status, on_sale)：前端首页「推荐 + 在售」feed（与 ES 同步条件一致）
--   - mall_card_pools(blind_box_id) UK：1:1 关系，pool 唯一归属
--   - mall_card_pool_items(pool_id, stock)：抽卡事务里按 pool_id + stock>0 选候选
--
-- 字段关键约束：
--   - mall_blind_boxes.supplier_id NOT NULL：纯多供应商模式，无平台自营盲盒。
--   - mall_card_pool_items.weight INT UNSIGNED：同池加和固定 10000（设计 D2），
--     用整数避免浮点累计误差。
--   - mall_card_pool_items.stock INT NOT NULL DEFAULT 0：抽卡事务内条件更新
--     `SET stock=stock-1 WHERE id=? AND stock>0`，保证不超卖。
--   - mall_card_pools.blind_box_id UK：pool 必须且只能属于一个 blind_box。

CREATE TABLE IF NOT EXISTS `mall_cards` (
  `id`          bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `name`        varchar(100) NOT NULL                                            COMMENT '卡名',
  `image`       varchar(255) DEFAULT NULL                                        COMMENT '卡图URL',
  `team`        varchar(64)  DEFAULT NULL                                        COMMENT '球队',
  `player`      varchar(64)  DEFAULT NULL                                        COMMENT '球员',
  `serial_no`   varchar(64)  DEFAULT NULL                                        COMMENT '编号/限量编号',
  `description` varchar(500) DEFAULT NULL                                        COMMENT '卡描述',
  `updated_at`  datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                      ON UPDATE CURRENT_TIMESTAMP(3)              COMMENT '更新时间',
  `created_at`  datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)            COMMENT '创建时间',
  `deleted_at`  datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_cards_team`   (`team`),
  KEY `idx_mall_cards_player` (`player`),
  KEY `idx_mall_cards_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mall_blind_boxes` (
  `id`          bigint          NOT NULL AUTO_INCREMENT                          COMMENT 'ID',
  `supplier_id` bigint          NOT NULL                                          COMMENT '所属供应商ID',
  `name`        varchar(100)    NOT NULL                                          COMMENT '盲盒名称',
  `cover`       varchar(255)    DEFAULT NULL                                      COMMENT '封面图URL',
  `price`       decimal(10,2)   NOT NULL DEFAULT 0                                COMMENT '原价(元)',
  `status`      varchar(16)     NOT NULL    DEFAULT 'active'                      COMMENT '状态(active=启用,disabled=禁用)',
  `on_sale`     tinyint(1)      NOT NULL    DEFAULT 1                             COMMENT '是否在售(1在售,0下架)',
  `is_featured` tinyint(1)      NOT NULL    DEFAULT 0                             COMMENT '是否推荐(1推荐,0普通)',
  `description` varchar(500)    DEFAULT NULL                                      COMMENT '盲盒描述',
  `updated_at`  datetime(3)     NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                            ON UPDATE CURRENT_TIMESTAMP(3)        COMMENT '更新时间',
  `created_at`  datetime(3)     NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)          COMMENT '创建时间',
  `deleted_at`  datetime(3)     DEFAULT NULL                                      COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_blind_boxes_supplier_status_onsale` (`supplier_id`, `status`, `on_sale`),
  KEY `idx_mall_blind_boxes_featured_status_onsale` (`is_featured`, `status`, `on_sale`),
  KEY `idx_mall_blind_boxes_deleted_at`            (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mall_card_pools` (
  `id`          bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `blind_box_id` bigint      NOT NULL                                            COMMENT '所属盲盒ID',
  `updated_at`  datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                      ON UPDATE CURRENT_TIMESTAMP(3)              COMMENT '更新时间',
  `created_at`  datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)            COMMENT '创建时间',
  `deleted_at`  datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_card_pools_blind_box` (`blind_box_id`),
  KEY `idx_mall_card_pools_deleted_at`      (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mall_card_pool_items` (
  `id`          bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `pool_id`     bigint       NOT NULL                                            COMMENT '所属卡池ID',
  `card_id`     bigint       NOT NULL                                            COMMENT '卡牌ID',
  `rarity`      varchar(8)   NOT NULL                                            COMMENT '稀有度(SSR/SR/R/N)',
  `weight`      int          NOT NULL    DEFAULT 0                               COMMENT '权重(同池加和=10000)',
  `stock`       int          NOT NULL    DEFAULT 0                               COMMENT '剩余库存',
  `updated_at`  datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                      ON UPDATE CURRENT_TIMESTAMP(3)              COMMENT '更新时间',
  `created_at`  datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)            COMMENT '创建时间',
  `deleted_at`  datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_card_pool_items_pool_stock` (`pool_id`, `stock`),
  KEY `idx_mall_card_pool_items_card`      (`card_id`),
  KEY `idx_mall_card_pool_items_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
