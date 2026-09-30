-- +migrate Up
--
-- admin_rule：菜单和权限规则表（参考 ba_admin_rule，按项目惯例重塑）。
-- 与 internal/model/admin.go 的 AdminRule 结构对齐：
--   - 时间戳字段按项目惯例补齐：user SQL 用 create_time / update_time（bigint UNSIGNED），
--     改为 created_at / updated_at（datetime(3) + GORM autoCreateTime / autoUpdateTime），
--     并补 deleted_at 软删除（CLAUDE.md 强制要求）；
--   - 枚举字段类型：user SQL 用 enum(...) → 改为 varchar(N)，由 Go 端自定义 string 类型
--     （AdminRuleType / AdminRuleOpenType / AdminRuleExtend）在应用层做约束（项目惯例，
--     与 add-config-model/design.md D2-D4 节保持一致；varchar 更易扩展新值）；
--   - 表名通过 config.yaml database.prefix 配置控制（这里写不带前缀的 admin_rule）；
--   - CHARSET / COLLATE 显式落到 utf8mb4 / utf8mb4_general_ci，与 baseline.up.sql 一致。

CREATE TABLE IF NOT EXISTS `admin_rule` (
  `id`         int UNSIGNED NOT NULL AUTO_INCREMENT                                  COMMENT 'ID',
  `pid`        int UNSIGNED NOT NULL DEFAULT 0                                       COMMENT '上级规则',
  `type`       varchar(16)  NOT NULL DEFAULT 'menu'                                  COMMENT '规则类型(dir=规则目录,menu=菜单项,node=权限节点)',
  `title`      varchar(50)  NOT NULL DEFAULT ''                                      COMMENT '规则标题',
  `name`       varchar(50)  NOT NULL DEFAULT ''                                      COMMENT '规则名称',
  `path`       varchar(100) NOT NULL DEFAULT ''                                      COMMENT '菜单路由路径',
  `icon`       varchar(50)  NOT NULL DEFAULT ''                                      COMMENT '图标',
  `open_type`  varchar(16)  DEFAULT NULL                                              COMMENT '菜单打开方式(tab=选项卡,link=链接,iframe=Iframe)',
  `url`        varchar(255) NOT NULL DEFAULT ''                                      COMMENT '菜单URL',
  `component`  varchar(100) NOT NULL DEFAULT ''                                      COMMENT '菜单组件路径',
  `keepalive`  tinyint(1)   NOT NULL DEFAULT 0                                       COMMENT '缓存(0=关闭,1=开启)',
  `extend`     varchar(32)  NOT NULL DEFAULT 'none'                                  COMMENT '扩展属性(none=无,add_route_only=只添加为路由,add_menu_only=只添加为菜单)',
  `remark`     varchar(255) NOT NULL DEFAULT ''                                      COMMENT '备注',
  `weigh`      int          NOT NULL DEFAULT 0                                       COMMENT '权重',
  `status`     tinyint      NOT NULL DEFAULT 1                                       COMMENT '状态(1启用,0禁用)',
  `updated_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
                            ON UPDATE CURRENT_TIMESTAMP(3)                             COMMENT '更新时间',
  `created_at` datetime(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3)                     COMMENT '创建时间',
  `deleted_at` datetime(3)  DEFAULT NULL                                              COMMENT '删除时间',
  PRIMARY KEY (`id`),
  KEY `idx_admin_rule_pid`        (`pid`),
  KEY `idx_admin_rule_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_general_ci;