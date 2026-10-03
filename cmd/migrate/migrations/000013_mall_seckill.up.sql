-- +migrate Up
--
-- 秒杀活动主体表（mall_seckill_activities）+ 库存扣减审计表（mall_stock_deduction_log）
-- 对应 add-trading-card-blindbox-mvp Phase 9 / D15。
--
-- 编号说明：原计划 000012，但 4.13 admin_rule_mall_menu 已占用并顺延到 000012，
-- 本迁移顺延到 000013（原预留给 Phase 9），后续 Phase 18 (follow_supplier) 用 000014。
-- 裁定见 openspec/tasks.md 任务 4.13 注释 + design D15。
--
-- mall_seckill_activities：
--   - redis_initialized 标志 Redis 库存是否已 init（创建活动时同事务 SET 防脏数据）。
--   - total_stock ≤ mall_card_pool_items 总 stock：service 层创建时校验（D15 决策点 2）。
--
-- mall_stock_deduction_log：
--   - synced_to_pool_at NULL = 未对账；cmd/stock-sync 异步对账仅写该字段，
--     实际 mall_card_pool_items.stock 在秒杀事务内已扣过（D15 步骤 4.b）。
--   - 索引 (synced_to_pool_at, deducted_at) 服务于"扫待对账"批量查询。

CREATE TABLE IF NOT EXISTS `mall_seckill_activities` (
  `id`               bigint      NOT NULL AUTO_INCREMENT                          COMMENT 'ID',
  `supplier_id`      bigint      NOT NULL                                          COMMENT '所属供应商ID',
  `blind_box_id`      bigint      NOT NULL                                          COMMENT '盲盒ID',
  `seckill_price`    decimal(10,2) NOT NULL DEFAULT 0                              COMMENT '秒杀价(元)',
  `total_stock`      int         NOT NULL DEFAULT 0                                COMMENT '秒杀总名额(用于初始化 Redis 库存)',
  `per_user_limit`   int         NOT NULL DEFAULT 1                                COMMENT '每用户限购数量',
  `start_at`         datetime(3) NOT NULL                                          COMMENT '活动开始时间',
  `end_at`           datetime(3) NOT NULL                                          COMMENT '活动结束时间',
  `status`           varchar(16) NOT NULL DEFAULT 'active'                         COMMENT '状态(active=启用,disabled=禁用)',
  `redis_initialized` tinyint(1) NOT NULL DEFAULT 0                                COMMENT 'Redis 库存是否已初始化(0=未初始化,1=已初始化)',
  `updated_at`       datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                                              ON UPDATE CURRENT_TIMESTAMP(3)      COMMENT '更新时间',
  `created_at`       datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)            COMMENT '创建时间',
  `deleted_at`       datetime(3) DEFAULT NULL                                      COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_seckill_status_window` (`status`, `start_at`, `end_at`),
  KEY `idx_mall_seckill_supplier`     (`supplier_id`),
  KEY `idx_mall_seckill_blind_box`    (`blind_box_id`),
  KEY `idx_mall_seckill_deleted_at`   (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mall_stock_deduction_log` (
  `id`                bigint        NOT NULL AUTO_INCREMENT                       COMMENT 'ID',
  `seckill_id`        bigint        NOT NULL                                       COMMENT '秒杀活动ID',
  `user_id`           bigint        NOT NULL                                       COMMENT '用户ID',
  `blind_box_id`      bigint        NOT NULL                                       COMMENT '盲盒ID',
  `card_id`           bigint        NOT NULL                                       COMMENT '抽中卡牌ID',
  `rarity`            varchar(8)    NOT NULL DEFAULT 'N'                           COMMENT '抽中等级(SSR/SR/R/N)',
  `snapshot_name`     varchar(100)  NOT NULL DEFAULT ''                            COMMENT '卡名快照',
  `snapshot_image`     varchar(255)  NOT NULL DEFAULT ''                            COMMENT '卡图快照',
  `deducted_at`       datetime(3)   NOT NULL                                       COMMENT '扣减时间',
  `synced_to_pool_at` datetime(3)   DEFAULT NULL                                   COMMENT '异步对账完成时间(NULL=未对账)',
  `created_at`        datetime(3)   NOT NULL DEFAULT CURRENT_TIMESTAMP(3)         COMMENT '创建时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_stock_log_synced_deducted` (`synced_to_pool_at`, `deducted_at`),
  KEY `idx_mall_stock_log_seckill`         (`seckill_id`),
  KEY `idx_mall_stock_log_user`            (`user_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;