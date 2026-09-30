-- +migrate Down
--
-- 回滚 admin_rule 表。该表无外键被引用，单独 DROP 安全。

DROP TABLE IF EXISTS `admin_rule`;