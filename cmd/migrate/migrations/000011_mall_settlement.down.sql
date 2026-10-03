-- +migrate Down

-- 删表（顺序：先 items → settlements → ledger）
DROP TABLE IF EXISTS `mall_settlement_items`;
DROP TABLE IF EXISTS `mall_settlements`;
DROP TABLE IF EXISTS `mall_platform_ledger`;

-- 撤销 mall_suppliers 结算字段
ALTER TABLE `mall_suppliers`
  DROP COLUMN `commission_rate`,
  DROP COLUMN `total_sales`,
  DROP COLUMN `balance`;
