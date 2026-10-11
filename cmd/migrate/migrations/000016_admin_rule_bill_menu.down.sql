-- +migrate Down
--
-- 回滚批次账单菜单种子（000016）。仅删除种子行（与 000012 既有约定一致）。

DELETE FROM `admin_rule`
WHERE `name` = 'admin/bills'
  AND `deleted_at` IS NULL;
