-- +migrate Up
--
-- users 表新增注册来源字段（add-uni-h5-client）。
--
-- 背景：H5 端手机号登录采用「首次登录自动建号」，建号时需记录来源，以便后续
-- 区分渠道（H5 手机号建号 / admin 预建 / 数据导入）。字段缺失时只能靠
-- password = '' 反推「无密码账号」，无法区分究竟是手机号建号还是其它来源。
--
-- 设计取舍：
--   - varchar(16)：现有取值 h5_mobile(9) / admin(5) / import(6)，留有余量；
--     不设 ENUM 或 CHECK 约束，避免新增渠道时再改表。
--   - DEFAULT NULL 且不加 NOT NULL：存量用户的来源不可考，NULL 语义为
--     「未知或历史数据」，比回填一个猜测值更诚实；存量回填属数据决策，
--     不在本次迁移中做。
--   - 追加在表尾（AFTER password）：不打扰既有列顺序与 ORM 字段假设，
--     保持最小侵入。
--   - 暂不建索引：该字段仅用于后台低频统计；若后续按来源的统计查询变频繁，
--     再补 idx_users_register_source。
--
-- 与 internal/model/user.go 的 User.RegisterSource 字段对齐。
-- 本期仅落 schema 与 model 字段；写入逻辑随 POST /user/login/mobile 的
-- 服务端实现落地（该接口实现属后续 change，不在本期范围内）。

ALTER TABLE `users`
  ADD COLUMN `register_source` varchar(16) DEFAULT NULL COMMENT '注册来源(h5_mobile/admin/import等,NULL=未知)' AFTER `password`;
