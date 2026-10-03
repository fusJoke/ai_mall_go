-- +migrate Up
--
-- admin_rule 菜单规则种子：插入 mall 业务的三组管理菜单
--   - 供应商管理  → /src/views/admin/supplier/index.vue
--   - 盲盒管理    → /src/views/admin/blindbox/index.vue
--   - 促销活动    → /src/views/admin/promotion/index.vue
-- 对应 add-trading-card-blindbox-mvp Phase 4.13 / 6.11。
--
-- 注意：本迁移原计划编号 000014（在 000013 follow_supplier 之后），
-- 因秒杀（Phase 9）/ 关注表（Phase 18）迁移尚未实施，取当前空闲编号 000012；
-- 后续 phase 的迁移编号顺延（见 .superpowers/sdd/tasks/progress.md 裁定）。
--
-- admin_rule.name 无唯一索引（见 000002），ON DUPLICATE KEY 不生效；
-- 用 INSERT ... SELECT ... WHERE NOT EXISTS 实现真正可重入。
-- pid=0 顶级菜单；status=1 启用；weigh 与既有 mall 外菜单一致为 0。

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,     `name`,            `path`,      `component`,                            `open_type`, `url`, `keepalive`, `extend`, `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
SELECT 0,        'menu', '供应商管理', 'admin/supplier', 'supplier', '/src/views/admin/supplier/index.vue',   NULL,        '',    0,           'none',   '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
WHERE NOT EXISTS (SELECT 1 FROM `admin_rule` WHERE `name` = 'admin/supplier' AND `deleted_at` IS NULL);

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,   `name`,           `path`,     `component`,                           `open_type`, `url`, `keepalive`, `extend`, `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
SELECT 0,        'menu', '盲盒管理', 'admin/blindbox', 'blindbox', '/src/views/admin/blindbox/index.vue', NULL,        '',    0,           'none',   '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
WHERE NOT EXISTS (SELECT 1 FROM `admin_rule` WHERE `name` = 'admin/blindbox' AND `deleted_at` IS NULL);

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,   `name`,            `path`,      `component`,                            `open_type`, `url`, `keepalive`, `extend`, `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
SELECT 0,        'menu', '促销活动', 'admin/promotion', 'promotion', '/src/views/admin/promotion/index.vue', NULL,        '',    0,           'none',   '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
WHERE NOT EXISTS (SELECT 1 FROM `admin_rule` WHERE `name` = 'admin/promotion' AND `deleted_at` IS NULL);
