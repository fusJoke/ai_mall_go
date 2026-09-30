-- +migrate Up
--
-- admin_group / admin_group_access：管理员分组表 + 管理员与分组的多对多关系表。
-- 与 internal/model/admin.go 的 AdminGroup / AdminGroupAccess 结构对齐：
--   - 时间戳字段按项目惯例补齐：user SQL 用 create_time / update_time（bigint UNSIGNED），
--     改为 created_at / updated_at（datetime(3) + GORM autoCreateTime / autoUpdateTime），
--     并补 deleted_at 软删除（CLAUDE.md 强制要求）；
--   - admin_group_access 复合主键 (uid, group_id)：user SQL 只声明 INDEX，按 design D3 升级为
--     PRIMARY KEY (uid, group_id)，表达"该管理员在该分组里"的精确关系；
--   - 表名通过 config.yaml database.prefix 配置控制（这里写不带前缀的 admin_group / admin_group_access）；
--   - CHARSET / COLLATE 显式落到 utf8mb4 / utf8mb4_general_ci，与 baseline.up.sql 一致。

-- ---------------------------------------------------------------------------
-- admin_group：管理员分组表（参考 ba_admin_group，按项目惯例重塑）。
--   - pid UNSIGNED NULL：与 AdminRule.Pid 默认 0 不同，本表 pid 允许 NULL 表达"无父节点"；
--   - rules text NULL：内容是 admin_rule.id 的逗号分隔列表，特殊值 '*' 表示全权限（超管）。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `admin_group` (
  `id`         int UNSIGNED NOT NULL AUTO_INCREMENT                                  COMMENT 'ID',
  `pid`        int UNSIGNED DEFAULT NULL                                              COMMENT '上级分组',
  `name`       varchar(100) NOT NULL DEFAULT ''                                      COMMENT '组名',
  `rules`      text         DEFAULT NULL                                              COMMENT '权限规则ID集',
  `status`     tinyint      NOT NULL DEFAULT 1                                       COMMENT '状态(1启用,0禁用)',
  `updated_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                            ON UPDATE CURRENT_TIMESTAMP(3)                             COMMENT '更新时间',
  `created_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)                     COMMENT '创建时间',
  `deleted_at` datetime(3)  DEFAULT NULL                                              COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_admin_group_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;

-- ---------------------------------------------------------------------------
-- admin_group_access：管理员与分组的映射表（关系表，复合主键）。
--   - 复合 PK (uid, group_id) 替代 user SQL 中的两个独立 INDEX；
--   - 保留 INDEX (uid) 与 INDEX (group_id) 用于"按 uid 反查" / "按 group_id 反查成员名单"。
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS `admin_group_access` (
  `uid`        int UNSIGNED NOT NULL                                                  COMMENT '管理员ID',
  `group_id`   int UNSIGNED NOT NULL                                                  COMMENT '分组ID',
  `updated_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                            ON UPDATE CURRENT_TIMESTAMP(3)                             COMMENT '更新时间',
  `created_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)                     COMMENT '创建时间',
  `deleted_at` datetime(3)  DEFAULT NULL                                              COMMENT '删除时间',
  PRIMARY KEY (`uid`, `group_id`),
  KEY `idx_admin_group_access_uid`        (`uid`),
  KEY `idx_admin_group_access_group_id`   (`group_id`),
  KEY `idx_admin_group_access_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;