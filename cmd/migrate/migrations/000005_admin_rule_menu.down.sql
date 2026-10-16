-- +migrate Down
--
-- 回滚 admin/rule 菜单规则种子：硬删除该行，与 000004 / 000012 / 000016 的 admin_rule
-- 种子回滚写法一致。admin_rule 虽有 deleted_at 软删列，但菜单种子属迁移产物，回滚即移除，
-- 不落软删痕迹；WHERE 里的 `deleted_at IS NULL` 只用于跳过已被软删的行。

DELETE FROM `admin_rule` WHERE `name` = 'admin/rule' AND `deleted_at` IS NULL;