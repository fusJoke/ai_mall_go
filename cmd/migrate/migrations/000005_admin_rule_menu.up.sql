-- +migrate Up
--
-- admin_rule 菜单规则种子：插入「菜单规则」菜单项（对应 add-admin-rule-management 引入的 /admin/rule 页面）。
-- 与 internal/model/admin.go 的 AdminRule 列对齐（见 000002_admin_rule.up.sql）。
--
-- 用 INSERT ... SELECT ... WHERE NOT EXISTS 保证幂等（与 000012 / 000016 的 admin_rule
-- 菜单种子写法一致）：
--   - name='admin/rule' 且未软删的行已存在 → 跳过；
--   - 否则插入新行。
--
-- 不要改回 ON DUPLICATE KEY UPDATE：admin_rule.name 上没有唯一索引（见 000002_admin_rule.up.sql
-- 的索引定义），该子句永远命中不了冲突分支，达不到注释所声称的「可重入」效果。
--
-- pid=0 表示顶级菜单（与 add-admin-management 的 admin/manager 平行）。
-- status=1 表示启用，与项目内 Admin.Status / AdminRule.Status 风格一致。

INSERT INTO `admin_rule`
  (`pid`, `type`,  `title`,    `name`,       `path`,  `component`,                    `open_type`, `url`, `keepalive`, `extend`, `remark`, `weigh`, `status`, `updated_at`,           `created_at`)
SELECT 0,        'menu', '菜单规则', 'admin/rule', 'rule', '/src/views/admin/rule/index.vue', NULL,        '',    0,           'none',   '',       0,       1,        CURRENT_TIMESTAMP(3), CURRENT_TIMESTAMP(3)
WHERE NOT EXISTS (SELECT 1 FROM `admin_rule` WHERE `name` = 'admin/rule' AND `deleted_at` IS NULL);