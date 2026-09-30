-- +migrate Up
--
-- admin_rule 菜单规则种子：插入「管理员账号」菜单项（对应 add-admin-management 引入的 /admin/manager 页面）。
-- 与 internal/model/admin.go 的 AdminRule 列对齐（见 000002_admin_rule.up.sql）。
--
-- 用 ON DUPLICATE KEY UPDATE 让迁移可重入：
--   - name='admin/manager' 已有 → 仅刷新 title；
--   - 不存在 → 插入新行。
--
-- pid=0 表示顶级菜单（后续若要归到「系统设置」分组再调整）。
-- status=1 表示启用，与项目内 Admin.Status / AdminRule.Status 风格一致。

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,        `name`,            `path`,      `component`,                       `open_type`, `url`, `keepalive`, `extend`,   `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
VALUES
  (0,     'menu', '管理员账号', 'admin/manager', 'manager', '/src/views/admin/manager/index.vue', NULL,        '',    0,           'none',     '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3))
ON DUPLICATE KEY UPDATE
  `title`      = VALUES(`title`),
  `path`       = VALUES(`path`),
  `component`  = VALUES(`component`),
  `updated_at` = CURRENT_TIMESTAMP(3);
