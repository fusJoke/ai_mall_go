-- +migrate Down
--
-- 回滚 000017：移除 users.register_source。
--
-- 注意：结构可回滚，但已写入的来源值会随之丢失且不可恢复；若生产环境已产生
-- 来源数据，回滚前应先导出该列。

ALTER TABLE `users` DROP COLUMN `register_source`;
