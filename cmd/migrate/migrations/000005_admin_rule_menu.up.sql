-- +migrate Up
--
-- admin_rule 菜单规则种子：插入「菜单规则」菜单项（对应 add-admin-rule-management 引入的 /admin/rule 页面）。
-- 与 internal/model/admin.go 的 AdminRule 列对齐（见 000002_admin_rule.up.sql）。
--
-- 用 ON DUPLICATE KEY UPDATE 让迁移可重入：
--   - name='admin/rule' 已有 → 仅刷新 title / path / component；
--   - 不存在 → 插入新行。
--
-- pid=0 表示顶级菜单（与 add-admin-management 的 admin/manager 平行）。
-- status=1 表示启用，与项目内 Admin.Status / AdminRule.Status 风格一致。

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,    `name`,       `path`,  `component`,                    `open_type`, `url`, `keepalive`, `extend`, `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
VALUES
  (0,     'menu', '菜单规则', 'admin/rule', 'rule', '/src/views/admin/rule/index.vue', NULL,        '',    0,           'none',   '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  `title`      = VALUES(`title`),
  `path`       = VALUES(`path`),
  `component`  = VALUES(`component`),
  `updated_at` = CURRENT_TIMESTAMP(3);