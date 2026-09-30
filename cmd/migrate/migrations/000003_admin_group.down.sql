-- +migrate Down
--
-- 回滚 admin_group / admin_group_access。建表逆序 DROP：先 drop 关系表（依赖主表），
-- 再 drop 主表。DROP TABLE IF EXISTS 允许幂等回滚。

DROP TABLE IF EXISTS `admin_group_access`;
DROP TABLE IF EXISTS `admin_group`;