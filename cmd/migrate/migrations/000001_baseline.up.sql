-- +migrate Up
--
-- baseline：把当前由 GORM AutoMigrate 维护的首批业务表落到 schema-迁移体系里。
-- schema_migrations 表由 golang-migrate 自维护（首次 up 自动创建），不需要在这里写。
--
-- 注意：
--   - 表结构与现有 GORM AutoMigrate 结果保持一致（命名 / charset / collation / 索引）。
--   - CREATE TABLE IF NOT EXISTS：baseline 对既有部署也是幂等的；如果某张表已由
--     GORM AutoMigrate 提前建好（既有部署的常见情况），再次执行 baseline 时会跳过；
--     字段差异不会被 baseline 修正，需要运维手工对齐（详见 design.md - Migration Plan）。
--   - CHARSET / COLLATE 显式落到 utf8mb4 / utf8mb4_general_ci，兼容 MySQL 5.7 / 8.0。

-- ---------------------------------------------------------------------------
-- users：通用测试用用户表（演示模型自注册 / AutoMigrate 写法）。
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
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- admins：管理员账号。
-- 与 internal/model/admin.go 的 gorm tag 字段顺序 / 类型 / 注释对齐。
--   - int → int, int8 → tinyint, int64 → bigint
--   - *string / *time.Time → DEFAULT NULL（gorm 指针语义）
--   - autoCreateTime / autoUpdateTime → DEFAULT CURRENT_TIMESTAMP(3) / ON UPDATE
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `admins` (
  `id`              bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `username`        varchar(64)  NOT NULL                                            COMMENT '用户名',
  `nickname`        varchar(64)  DEFAULT NULL                                        COMMENT '昵称',
  `avatar`          varchar(255) DEFAULT NULL                                        COMMENT '头像URL',
  `email`           varchar(128) DEFAULT NULL                                        COMMENT '邮箱',
  `mobile`          varchar(20)  DEFAULT NULL                                        COMMENT '手机号',
  `login_failure`   int          NOT NULL    DEFAULT 0                               COMMENT '登录失败次数',
  `last_login_at`   datetime(3)  DEFAULT NULL                                        COMMENT '最后登录时间',
  `last_login_ip`   varchar(45)  DEFAULT NULL                                        COMMENT '最后登录IP',
  `password`        varchar(255) NOT NULL                                            COMMENT '密码(已哈希)',
  `bio`             varchar(500) DEFAULT NULL                                        COMMENT '个人简介',
  `status`          tinyint      NOT NULL    DEFAULT 1                               COMMENT '状态(1启用,0禁用)',
  `updated_at`      datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                         ON UPDATE CURRENT_TIMESTAMP(3)               COMMENT '更新时间',
  `created_at`      datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)             COMMENT '创建时间',
  `deleted_at`      datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_admins_username`    (`username`),
  UNIQUE KEY `idx_admins_email`       (`email`),
  UNIQUE KEY `idx_admins_mobile`      (`mobile`),
  KEY        `idx_admins_deleted_at`  (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- tokens：API 调用凭证（登录会话 / refresh token）。
-- 与 internal/model/common.go 的 Token 结构对齐。
--   - composite index idx_user_type (UserID 优先, Type 次之)
--   - Token 字段为 SHA256 hex 长度 64，用 size:128 冗余
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `tokens` (
  `id`         bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
  `token`      varchar(128) NOT NULL                                            COMMENT '令牌哈希(SHA256)',
  `type`       varchar(32)  NOT NULL                                            COMMENT '令牌类型',
  `user_id`    bigint       NOT NULL                                            COMMENT '用户ID',
  `expires_at` datetime(3)  NOT NULL                                            COMMENT '过期时间',
  `updated_at` datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                     ON UPDATE CURRENT_TIMESTAMP(3)               COMMENT '更新时间',
  `created_at` datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)             COMMENT '创建时间',
  `deleted_at` datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_tokens_token`      (`token`),
  KEY        `idx_user_type`         (`user_id`, `type`),
  KEY        `idx_tokens_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- captchas：验证码表（一次性）。
-- 与 internal/model/captcha.go 的 Captcha 结构对齐：字符串 PK，无 UpdatedAt / 无软删除。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `captchas` (
  `key`         varchar(64) NOT NULL                                  COMMENT '验证令牌',
  `code`        varchar(255) DEFAULT NULL                              COMMENT '验证码值(已加密,文本类使用)',
  `info`        text        DEFAULT NULL                              COMMENT '验证码详情JSON(点选类使用)',
  `expires_at`  datetime(3) NOT NULL                                   COMMENT '过期时间',
  `created_at`  datetime(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)      COMMENT '创建时间',
  PRIMARY KEY (`key`),
  KEY `idx_captchas_expires_at` (`expires_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- config：站点级键值型配置（站点名称 / 备案号 / 客服联系方式等）。
-- 与 internal/model/config.go 的 Config 结构对齐：name 唯一键，其余字段允许空字符串默认。
-- Value / Content 用 longtext（GORM tag: type:longtext）。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `config` (
  `id`           bigint       NOT NULL AUTO_INCREMENT                            COMMENT 'ID',
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
  `allow_del`    tinyint(1)   NOT NULL    DEFAULT 0                               COMMENT '允许删除',
  `weigh`        int          NOT NULL    DEFAULT 0                               COMMENT '权重',
  `updated_at`   datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)
                                       ON UPDATE CURRENT_TIMESTAMP(3)             COMMENT '更新时间',
  `created_at`   datetime(3)  NOT NULL    DEFAULT CURRENT_TIMESTAMP(3)           COMMENT '创建时间',
  `deleted_at`   datetime(3)  DEFAULT NULL                                        COMMENT '删除时间',
  PRIMARY KEY (`id`),
  UNIQUE KEY `idx_config_name`        (`name`),
  KEY        `idx_config_deleted_at`  (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;


