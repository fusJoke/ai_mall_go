-- +migrate Down
--
-- 回滚 000006_mall_users.up.sql：
--   - 删 (username) / (mobile) / (email) 索引
--   - 重建 email 唯一约束
--   - 删除 password / login_failure / last_login_ip / last_login_at /
--     status / balance / mobile / avatar / nickname / email 列
--   - username 重命名回 name（去掉 COMMENT）

ALTER TABLE `users`
  DROP INDEX `idx_users_username`,
  DROP INDEX `idx_users_mobile`,
  DROP INDEX `idx_users_email`;

ALTER TABLE `users`
  ADD UNIQUE KEY `idx_users_email` (`email`),
  DROP COLUMN `password`,
  DROP COLUMN `login_failure`,
  DROP COLUMN `last_login_ip`,
  DROP COLUMN `last_login_at`,
  DROP COLUMN `status`,
  DROP COLUMN `balance`,
  DROP COLUMN `mobile`,
  DROP COLUMN `avatar`,
  DROP COLUMN `nickname`,
  DROP COLUMN `email`;

ALTER TABLE `users`
  CHANGE COLUMN `username` `name` varchar(64) NOT NULL AFTER `id`;
