-- +migrate Up
--
-- mall_suppliers + mall_supplier_users：供应商主体 + 供应商登录账号。
--   - mall_suppliers：name / logo / bio / status(active|disabled) / is_featured /
--     contact_phone；结算字段 balance / total_sales / commission_rate 由
--     000011_mall_settlement 追加，避免单迁移承担太多语义。
--   - mall_supplier_users：每个供应商唯一登录账号（UK supplier_id），
--     username 全局唯一（UK username）。
--
-- 索引说明：
--   - idx_mall_suppliers(status, is_featured)：admin 端按"启用 + 推荐"筛选。
--   - idx_mall_supplier_users_supplier_id UK：1:1 关系，supplier 唯一账号。
--   - idx_mall_supplier_users_username UK：登录用，全局唯一。
--
-- status 字段取值说明：
--   - mall_suppliers.status 用 varchar('active' / 'disabled')：业务状态比
--     通用启停更明确，"禁用" 表达"被 admin 屏蔽" 语义，对应设计 D9
--     「供应商状态与盲盒状态解耦」。
--   - mall_supplier_users.status 用 tinyint(1/0)：与 Admin.Status /
--     AdminRule.Status 风格一致，单账号级启停。

CREATE TABLE IF NOT EXISTS `mall_suppliers` (
  `id`              bigint       NOT NULL AUTO_INCREMENT                           COMMENT 'ID',
  `name`            varchar(100) NOT NULL                                           COMMENT '供应商名称',
  `logo`            varchar(255) DEFAULT NULL                                       COMMENT 'LOGO URL',
  `bio`             varchar(500) DEFAULT NULL                                       COMMENT '简介',
  `status`          varchar(16)  NOT NULL    DEFAULT 'active'                       COMMENT '状态(active=启用,disabled=禁用)',
  `is_featured`     tinyint(1)   NOT NULL    DEFAULT 0                              COMMENT '是否推荐(1推荐,0普通)',
  `contact_phone`   varchar(20)  DEFAULT NULL                                       COMMENT '联系电话',
  `updated_at`      datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                          ON UPDATE CURRENT_TIMESTAMP(3)           COMMENT '更新时间',
  `created_at`      datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)           COMMENT '创建时间',
  `deleted_at`      datetime(3)  DEFAULT NULL                                       COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_mall_suppliers_status_featured` (`status`, `is_featured`),
  KEY `idx_mall_suppliers_deleted_at`      (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

CREATE TABLE IF NOT EXISTS `mall_supplier_users` (
  `id`              bigint       NOT NULL AUTO_INCREMENT                           COMMENT 'ID',
  `supplier_id`     bigint       NOT NULL                                           COMMENT '所属供应商ID',
  `username`        varchar(64)  NOT NULL                                           COMMENT '登录用户名',
  `password`        varchar(255) NOT NULL                                           COMMENT '密码(已哈希)',
  `status`          tinyint      NOT NULL    DEFAULT 1                              COMMENT '状态(1启用,0禁用)',
  `last_login_at`   datetime(3)  DEFAULT NULL                                       COMMENT '最后登录时间',
  `last_login_ip`   varchar(45)  DEFAULT NULL                                       COMMENT '最后登录IP',
  `updated_at`      datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                          ON UPDATE CURRENT_TIMESTAMP(3)           COMMENT '更新时间',
  `created_at`      datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)           COMMENT '创建时间',
  `deleted_at`      datetime(3)  DEFAULT NULL                                       COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_mall_supplier_users_supplier` (`supplier_id`),
  UNIQUE KEY `idx_mall_supplier_users_username` (`username`),
  KEY        `idx_mall_supplier_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;
