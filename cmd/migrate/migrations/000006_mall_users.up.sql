-- +migrate Up
--
-- 扩展 users 表为 mall_users（沿用现有 users 表做最小侵入）：
--   - name 重命名为 username（语义对齐 Admin.Username）
--   - 加 nickname / avatar / mobile / email / balance / status / last_login_at /
--     last_login_ip / login_failure / password 字段
--   - email 不再唯一（允许 NULL 与重复），由 idx_users_email 普通索引覆盖查找
--   - 新增 (username) UK / (mobile) UK 索引
--
-- 与 internal/model/mall/user.go 的 MallUser 字段顺序 / 类型 / 注释对齐。
--
-- 设计取舍：
--   - 不新建 mall_users 表（避免双表并存 + 数据迁移），直接扩展 users；
--     现有 users 表仅是 AutoMigrate 占位，无真实数据，可安全 rename 列。
--   - balance 用 decimal(12,2) 而非 bigint：抽卡单价按「元」计，金额类
--     字段统一用定点小数，避免浮点误差。decimal(12,2) 上限 9,999,999,999.99
--     远超单用户余额上限。
--   - status 用 tinyint 与 Admin.Status / AdminRule.Status 风格一致；
--     1=启用 / 0=禁用。
--   - password 字段命名沿用 Admin.Password 而非 proposal 中提到的
--     "password_hash"，因为 GORM 解析时列名直接等于字段名 → password，
--     由代码侧文档化「已哈希」语义；hash 算法在 service 层选型。

ALTER TABLE `users`
  CHANGE COLUMN `name` `username` varchar(64) NOT NULL COMMENT '用户名' AFTER `id`;

ALTER TABLE `users`
  ADD COLUMN `nickname`        varchar(64)  DEFAULT NULL                                COMMENT '昵称'             AFTER `username`,
  ADD COLUMN `avatar`          varchar(255) DEFAULT NULL                                COMMENT '头像URL'           AFTER `nickname`,
  ADD COLUMN `mobile`          varchar(20)  DEFAULT NULL                                COMMENT '手机号'            AFTER `avatar`,
  ADD COLUMN `balance`         decimal(12,2) NOT NULL DEFAULT 0                         COMMENT '余额(元)'          AFTER `email`,
  ADD COLUMN `status`          tinyint      NOT NULL DEFAULT 1                         COMMENT '状态(1启用,0禁用)'  AFTER `balance`,
  ADD COLUMN `last_login_at`   datetime(3)  DEFAULT NULL                                COMMENT '最后登录时间'       AFTER `status`,
  ADD COLUMN `last_login_ip`   varchar(45)  DEFAULT NULL                                COMMENT '最后登录IP'         AFTER `last_login_at`,
  ADD COLUMN `login_failure`   int          NOT NULL DEFAULT 0                         COMMENT '登录失败次数'       AFTER `last_login_ip`,
  ADD COLUMN `password`        varchar(255) NOT NULL DEFAULT ''                        COMMENT '密码(已哈希)'       AFTER `login_failure`;

-- email 列在 baseline（000001）已存在，且当时是 `varchar(128) NOT NULL` + UNIQUE KEY。
-- 因此这里只能改属性，不能重复 ADD COLUMN（MySQL 会报 ERROR 1060 Duplicate column name）：
-- 去掉 NOT NULL，对应 spec「email 不再唯一（允许 NULL 与重复）」。
ALTER TABLE `users`
  MODIFY COLUMN `email` varchar(128) DEFAULT NULL COMMENT '邮箱' AFTER `mobile`;

-- 删除原 idx_users_email 唯一约束 → email 不再唯一
ALTER TABLE `users` DROP INDEX `idx_users_email`;

-- 新增索引：
--   - idx_users_username 唯一
--   - idx_users_mobile 唯一（mobile 列允许 NULL，MySQL 允许多 NULL 共存）
--   - idx_users_email 普通（保留查找能力，去掉唯一约束）
ALTER TABLE `users`
  ADD UNIQUE KEY `idx_users_username` (`username`),
  ADD UNIQUE KEY `idx_users_mobile` (`mobile`),
  ADD KEY        `idx_users_email` (`email`);
