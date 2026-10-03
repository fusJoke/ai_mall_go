-- +migrate Down
--
-- 回滚批次结算迁移（000015）。
-- 先删明细表再删主表（外键虽未强制，保持依赖顺序习惯）。

DROP TABLE IF EXISTS `mall_settlement_bill_items`;
DROP TABLE IF EXISTS `mall_settlement_bills`;

ALTER TABLE `mall_blind_boxes`
  DROP INDEX `idx_mall_blind_boxes_supplier_lifecycle`,
  DROP COLUMN `lifecycle`;
