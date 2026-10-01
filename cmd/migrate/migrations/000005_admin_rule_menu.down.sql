-- +migrate Down
--
-- 回滚 admin/rule 菜单规则。仅软删（与基线约定一致），保留审计痕迹。

DELETE FROM `admin_rule` WHERE `name` = 'admin/rule' AND `deleted_at` IS NULL;