package mall

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

// 「模型 ↔ 迁移」schema 一致性测试（OCR 审查 P2-1 的防复发措施）。
//
// 背景：本项目的 `database.Init()` 明确**不再调用 AutoMigrate**（见
// internal/infra/database/database.go），表结构完全由 cmd/migrate/migrations 下的
// SQL 决定。于是 GORM 结构体上的标签只剩「文档」作用 —— 它写错了既不会改表、
// 也不会报错，只会持续误导读者，直到有人在错误前提下写业务逻辑。
//
// 已发生的实例：mall_settlement_bills 的模型把 (supplier_id, status) 标成
// uniqueIndex，而迁移 000015 建的是普通 KEY (supplier_id, status, created_at)。
// 「一个供应商同一状态只能有一张账单」在业务上显然不成立（同一供应商的多个商品
// 会各自生成 generated 账单）。本测试把这类漂移变成 CI 可见的失败。
//
// 比较口径：只比较**两边都存在**的同名索引的唯一性与列序 —— 这是漂移的实质。
// 只在一边存在的索引不判失败（模型可能用标签声明了别名，迁移可能出于 DBA 考虑加了
// 额外索引），否则测试会过于脆弱。

// migrationIndex 是迁移 SQL 里的一条索引定义。
type migrationIndex struct {
	Unique  bool
	Columns []string
}

var (
	// 抓 CREATE TABLE 的「表名 + 列/索引定义块」。
	createTableRe = regexp.MustCompile("(?is)CREATE TABLE(?: IF NOT EXISTS)?\\s+`(\\w+)`\\s*\\((.*?)\\)\\s*ENGINE")

	// 抓索引行：`UNIQUE KEY name (cols)` 或 `KEY name (cols)`。
	indexLineRe = regexp.MustCompile("(?im)^\\s*(UNIQUE\\s+KEY|KEY)\\s+`([^`]+)`\\s*\\(([^)]*)\\)")

	// 抓列名（反引号内），用于拆索引列列表。
	columnRe = regexp.MustCompile("`([^`]+)`")
)

// parseMigrationIndexes 从迁移 SQL 文本里抽出指定表的索引定义。
func parseMigrationIndexes(t *testing.T, sqlText, table string) map[string]migrationIndex {
	t.Helper()

	var body string
	for _, m := range createTableRe.FindAllStringSubmatch(sqlText, -1) {
		if m[1] == table {
			body = m[2]
			break
		}
	}
	if body == "" {
		t.Fatalf("在迁移 SQL 中找不到 CREATE TABLE `%s`", table)
	}

	out := map[string]migrationIndex{}
	for _, m := range indexLineRe.FindAllStringSubmatch(body, -1) {
		name := m[2]
		cols := make([]string, 0, 4)
		for _, c := range columnRe.FindAllStringSubmatch(m[3], -1) {
			cols = append(cols, c[1])
		}
		// 列定义里可能带 DESC / 长度，如 `created_at` DESC → 只取列名。
		out[name] = migrationIndex{
			Unique:  strings.HasPrefix(strings.ToUpper(strings.TrimSpace(m[1])), "UNIQUE"),
			Columns: cols,
		}
	}
	return out
}

// modelIndexes 用 GORM 自己的解析器读出结构体标签声明的索引。
func modelIndexes(t *testing.T, dest any) map[string]migrationIndex {
	t.Helper()

	parsed, err := schema.Parse(dest, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("schema.Parse: %v", err)
	}

	out := map[string]migrationIndex{}
	for _, idx := range parsed.ParseIndexes() {
		cols := make([]string, 0, len(idx.Fields))
		for _, f := range idx.Fields {
			// Expression 索引（函数索引）没有对应列名，用原始表达式占位以便报错可读。
			if f.Field != nil {
				cols = append(cols, f.Field.DBName)
			} else {
				cols = append(cols, f.Expression)
			}
		}
		out[idx.Name] = migrationIndex{
			Unique:  strings.EqualFold(idx.Class, "UNIQUE"),
			Columns: cols,
		}
	}
	return out
}

// repoRoot 通过 runtime.Caller 定位仓库根（测试文件在 internal/model/mall/ 下）。
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

func TestModelIndexesMatchMigrations(t *testing.T) {
	root := repoRoot(t)

	cases := []struct {
		table     string
		model     any
		migration string
	}{
		{"mall_settlement_bills", &MallSettlementBill{}, "000015_mall_batch_settlement.up.sql"},
		{"mall_settlement_bill_items", &MallSettlementBillItem{}, "000015_mall_batch_settlement.up.sql"},
	}

	for _, tc := range cases {
		t.Run(tc.table, func(t *testing.T) {
			path := filepath.Join(root, "cmd", "migrate", "migrations", tc.migration)
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("读取迁移文件: %v", err)
			}
			migIdx := parseMigrationIndexes(t, string(raw), tc.table)
			modIdx := modelIndexes(t, tc.model)

			if len(migIdx) == 0 {
				t.Fatalf("迁移里没解析出任何索引，解析器可能失效")
			}

			compared := 0
			for name, mi := range migIdx {
				modelI, ok := modIdx[name]
				if !ok {
					continue
				}
				compared++
				if mi.Unique != modelI.Unique {
					t.Errorf(
						"索引 %s 唯一性漂移：迁移 unique=%v，模型 unique=%v\n"+
							"  （AutoMigrate 已停用 → 模型标签写错不会改表也不会报错，只会误导读者）",
						name, mi.Unique, modelI.Unique,
					)
				}
				if strings.Join(mi.Columns, ",") != strings.Join(modelI.Columns, ",") {
					t.Errorf(
						"索引 %s 列不一致：迁移 %v，模型 %v",
						name, mi.Columns, modelI.Columns,
					)
				}
			}
			if compared == 0 {
				t.Errorf("模型与迁移没有任何同名索引可比较，测试失去意义")
			}
		})
	}
}

// TestSettlementBillSupplierStatusIndexIsNotUnique 直接锁定 P2-1 的具体回归：
// (supplier_id, status, created_at) 必须是普通索引。
//
// 让这条断言独立存在，是因为它是本次漂移的原始现场；万一有人把一致性测试的比较
// 口径改宽（例如跳过唯一性差异），本条仍会红。
func TestSettlementBillSupplierStatusIndexIsNotUnique(t *testing.T) {
	const name = "idx_mall_settlement_bills_supplier_status"

	idx, ok := modelIndexes(t, &MallSettlementBill{})[name]
	if !ok {
		t.Fatalf("模型里找不到索引 %s", name)
	}
	if idx.Unique {
		t.Errorf(
			"%s 被声明为 UNIQUE —— 但业务唯一键是 blind_box_id（1 商品 1 账单），"+
				"同一供应商在同一状态下必然同时存在多张账单（P2-1）", name,
		)
	}
	want := "supplier_id,status,created_at"
	if got := strings.Join(idx.Columns, ","); got != want {
		t.Errorf("%s 列 = %s, want %s（与迁移 000015 对齐）", name, got, want)
	}
}
