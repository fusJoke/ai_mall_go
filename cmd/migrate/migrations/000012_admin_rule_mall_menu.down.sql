-- +migrate Down
--
-- 回滚 mall 业务的三组管理菜单。仅删除种子行（与既有约定一致）。

DELETE FROM `admin_rule`
WHERE `name` IN ('admin/supplier', 'admin/blindbox', 'admin/promotion')
  AND `deleted_at` IS NULL;
