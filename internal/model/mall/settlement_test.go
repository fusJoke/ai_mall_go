package mall

import (
	"testing"

	"gorm.io/gorm/schema"
)

// ============================================================
// 测试 Model 层的"非业务字段"：
//   - TableName 必须返回带前缀的表名
//   - 状态枚举值稳定（字符串字面量写进 MySQL，被埋点 / 报表依赖）
// ============================================================

// TestSettlementStatus_Constants 锁住 status 字面量，避免拼写漂移。
//
// 这些字面量是 admin 后台 / 报表系统读取 SQL `status = 'pending'` 的依据，
// 重命名会破坏"老数据 + 新代码"的兼容 —— 测试守住契约。
func TestSettlementStatus_Constants(t *testing.T) {
	cases := []struct {
		got  MallSettlementStatus
		want string
	}{
		{SettlementStatusPending, "pending"},
		{SettlementStatusProcessing, "processing"},
		{SettlementStatusPaid, "paid"},
		{SettlementStatusFailed, "failed"},
	}
	for _, c := range cases {
		if string(c.got) != c.want {
			t.Errorf("status = %q, want %q", string(c.got), c.want)
		}
	}
}

// TestTableName_WithPrefix 验证 MallSettlement / MallSettlementItem / MallPlatformLedger
// 在带 prefix 时返回带前缀的表名（与 naming strategy 配合）。
//
// 设计：D8 / D9 的 MallSettlement 系列都走 `mall_` 前缀；如果未来迁移到无前缀或
// 其他前缀，本测试会失败 —— 提示开发者同步 SQL 迁移脚本 / 数据库适配。
//
// 依赖：internal/infra/database/... 设置了 `SingularTable: true`，
// 所以 namer 不会自动加 s —— 返回的是 "mall_mall_xxx" 单数形式。
func TestTableName_WithPrefix(t *testing.T) {
	cfg := &schema.NamingStrategy{TablePrefix: "mall_", SingularTable: true}

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"MallSettlement", (MallSettlement{}).TableName(cfg), "mall_mall_settlements"},
		{"MallSettlementItem", (MallSettlementItem{}).TableName(cfg), "mall_mall_settlement_items"},
		{"MallPlatformLedger", (MallPlatformLedger{}).TableName(cfg), "mall_mall_platform_ledger"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s TableName = %q, want %q", c.name, c.got, c.want)
		}
	}
}

// TestTableName_NoPrefix 验证无 prefix 时表名不带前缀（避免遗留代码误加）。
func TestTableName_NoPrefix(t *testing.T) {
	cfg := &schema.NamingStrategy{TablePrefix: "", SingularTable: true}
	settlement := MallSettlement{}
	if got := settlement.TableName(cfg); got != "mall_settlements" {
		t.Errorf("no-prefix TableName = %q, want mall_settlements", got)
	}
}