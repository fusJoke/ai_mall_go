-- +migrate Up
--
-- baseline：把当前由 GORM AutoMigrate 维护的首批业务表落到 schema-迁移体系里。
-- schema_migrations 表由 golang-migrate 自维护（首次 up 自动创建），不需要在这里写。
--
-- 设计原则：
--   - 表结构与现有 GORM AutoMigrate 输出 **字节级一致**。这是已被现有 MySQL 实例"快照"
--     过的 schema（见 scripts/dump_gorm_schema），从 MySQL information_schema 离线读出。
--   - 命名 / charset / collation / 索引逐一对齐。字段类型走 GORM 的 Go→MySQL 默认映射：
--       int64 → bigint, int → bigint, int8 → tinyint, uint → bigint unsigned,
--       bool → tinyint(1), *string / *time.Time → DEFAULT NULL, time.Time 默认 NOT NULL 无 default。
--   - 字符序全部 utf8mb4 / utf8mb4_unicode_ci（与 production MySQL 8.0 一致）。
--   - default 数值 GORM 用字符串常量（'0' / '1'）输出，照搬。
--   - autoCreateTime / autoUpdateTime 由应用层填写，**不**加 DEFAULT CURRENT_TIMESTAMP，
--     也不加 ON UPDATE CURRENT_TIMESTAMP —— 与现有 DB 完全一致。
--   - CREATE TABLE IF NOT EXISTS：baseline 对既有部署也是幂等的；差异不会被 baseline
--     修正（spec 的明确取舍 —— 加列操作由后续 migration 负责，不在本 baseline 范围）。

-- ---------------------------------------------------------------------------
-- admins：管理员账号（internal/model/admin.go）。
-- GORM 已建表，schema 源自 information_schema dump。
--   - login_failure int → bigint (Go `int` 映射)
--   - status int8 → tinyint
--   - 所有字符串唯一键 (username/email/mobile)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `admins` (
  `id`              bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `username`        varchar(64)  NOT NULL    DEFAULT ''                              COMMENT '用户名',
  `nickname`        varchar(64)  DEFAULT NULL                                        COMMENT '昵称',
  `avatar`          varchar(255) DEFAULT NULL                                        COMMENT '头像URL',
  `email`           varchar(128) DEFAULT NULL                                        COMMENT '邮箱',
  `mobile`          varchar(20)  DEFAULT NULL                                        COMMENT '手机号',
  `login_failure`   bigint       NOT NULL    DEFAULT '0'                             COMMENT '登录失败次数',
  `last_login_at`   datetime(3)  DEFAULT NULL                                        COMMENT '最后登录时间',
  `last_login_ip`   varchar(45)  DEFAULT NULL                                        COMMENT '最后登录IP',
  `password`        varchar(255) NOT NULL                                            COMMENT '密码(已哈希)',
  `bio`             varchar(500) DEFAULT NULL                                        COMMENT '个人简介',
  `status`          tinyint      NOT NULL    DEFAULT '1'                             COMMENT '状态(1启用,0禁用)',
  `updated_at`      datetime(3)  NOT NULL                                            COMMENT '更新时间',
  `created_at`      datetime(3)  NOT NULL                                            COMMENT '创建时间',
  `deleted_at`      datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_admins_username`    (`username`),
  UNIQUE KEY `idx_admins_email`       (`email`),
  UNIQUE KEY `idx_admins_mobile`      (`mobile`),
  KEY        `idx_admins_deleted_at`  (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- tokens：API 调用凭证（internal/model/common.go）。
-- 唯一索引 idx_tokens_token、复合索引 idx_user_type (user_id, type)、软删除索引。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `tokens` (
  `id`         bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `token`      varchar(128) NOT NULL                                            COMMENT '令牌哈希(SHA256)',
  `type`       varchar(32)  NOT NULL                                            COMMENT '令牌类型',
  `user_id`    bigint       NOT NULL                                            COMMENT '用户ID',
  `expires_at` datetime(3)  NOT NULL                                            COMMENT '过期时间',
  `updated_at` datetime(3)  NOT NULL                                            COMMENT '更新时间',
  `created_at` datetime(3)  NOT NULL                                            COMMENT '创建时间',
  `deleted_at` datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_tokens_token`      (`token`),
  KEY        `idx_user_type`         (`user_id`, `type`),
  KEY        `idx_tokens_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- captchas：验证码表（internal/model/captcha.go）。
-- 字符串 PK（key），无 UpdatedAt / 无软删除；ExpiresAt 索引。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `captchas` (
  `key`         varchar(64) NOT NULL                                            COMMENT '验证令牌',
  `code`        varchar(255) DEFAULT NULL                                       COMMENT '验证码值(已加密,文本类使用)',
  `info`        text        DEFAULT NULL                                        COMMENT '验证码详情JSON(点选类使用)',
  `expires_at`  datetime(3) NOT NULL                                            COMMENT '过期时间',
  `created_at`  datetime(3) NOT NULL                                            COMMENT '创建时间',
  PRIMARY KEY (`key`),
  KEY `idx_captchas_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- config：站点级键值型配置（internal/model/config.go）。
--   - ID uint → bigint unsigned
--   - Weigh int → bigint
--   - AllowDel bool → tinyint(1) DEFAULT '0'
--   - Value / Content 是 *string，longtext DEFAULT NULL
--   - name 唯一键，deleted_at 索引
-- 注意：本次本地预存的 GORM 版本里 `config` 表**不存在**（可能历史上手工 drop 过）；
-- baseline 会把表补齐，与 GORM AutoMigrate 重跑后的产物对齐。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `config` (
  `id`           bigint unsigned NOT NULL AUTO_INCREMENT                          COMMENT 'ID',
  `name`         varchar(30)  NOT NULL                                            COMMENT '变量名',
  `group`        varchar(30)  NOT NULL    DEFAULT ''                              COMMENT '分组',
  `title`        varchar(50)  NOT NULL    DEFAULT ''                              COMMENT '变量标题',
  `tip`          varchar(100) NOT NULL    DEFAULT ''                              COMMENT '变量描述',
  `type`         varchar(30)  NOT NULL    DEFAULT ''                              COMMENT '变量输入组件类型',
  `value`        longtext     DEFAULT NULL                                        COMMENT '变量值',
  `content`      longtext     DEFAULT NULL                                        COMMENT '字典数据',
  `rule`         varchar(100) NOT NULL    DEFAULT ''                              COMMENT '验证规则',
  `extend`       varchar(255) NOT NULL    DEFAULT ''                              COMMENT '扩展属性',
  `input_extend` varchar(255) NOT NULL    DEFAULT ''                              COMMENT '输入框扩展属性',
  `allow_del`    tinyint(1)   NOT NULL    DEFAULT '0'                             COMMENT '允许删除',
  `weigh`        bigint       NOT NULL    DEFAULT '0'                             COMMENT '权重',
  `updated_at`   datetime(3)  NOT NULL                                            COMMENT '更新时间',
  `created_at`   datetime(3)  NOT NULL                                            COMMENT '创建时间',
  `deleted_at`   datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_config_name`        (`name`),
  KEY        `idx_config_deleted_at`  (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- ---------------------------------------------------------------------------
-- users：通用测试用用户表（internal/model/user.go）。
--   - 字段无 comment（struct 中没显式 comment tag），保留与 GORM 输出对齐。
--   - 字段顺序：id / name / email / created_at / updated_at / deleted_at（按 struct 顺序）。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `users` (
  `id`         bigint       NOT NULL AUTO_INCREMENT,
  `name`       varchar(64)  NOT NULL,
  `email`      varchar(128) NOT NULL,
  `created_at` datetime(3)  NOT NULL,
  `updated_at` datetime(3)  NOT NULL,
  `deleted_at` datetime(3)  DEFAULT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_users_email` (`email`),
  KEY        `idx_users_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
