-- +migrate Down
--
-- baseline 回滚：把首批业务表按 创建顺序的逆序 DROP。
-- schema_migrations 表由 golang-migrate 自维护，**不要** DROP；这里只 DROP 业务表。
--
-- DROP TABLE IF EXISTS 允许幂等回滚；先 down 到 0 再 up 一次会自动重新建表。

DROP TABLE IF EXISTS `config`;
DROP TABLE IF EXISTS `captchas`;
DROP TABLE IF EXISTS `tokens`;
DROP TABLE IF EXISTS `admins`;
DROP TABLE IF EXISTS `users`;
