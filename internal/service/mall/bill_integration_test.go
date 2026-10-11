//go:build integration

// Package mall 批次结算端到端集成测试（task 8.2）。
//
// 跑通「商品即批次 v2」完整闭环：
//
//	上架商品 → 连续抽卡至售罄 → 售罄那一单必须出现在账单明细
//	→ 供应商确认(generated→confirmed) → admin 打款(confirmed→paid)
//	→ 商品置 settled + supplier.balance 扣减正确
//
// 与 internal/service/admin/settlement_integration_test.go 的区别：那个覆盖的是
// **旧的周期结算**（admin 手动按周期 generate），本文件覆盖**批次账单**路径
// （draw 事务尾部 7.7 自动售罄检测 → FinalizeSoldOut → 自动建账单）。
//
// 必须挂 //go:build integration build tag，默认 `go test ./...` 不跑。
// 运行方式（**必须**用专用测试库，库名含 "test"；测试会 DROP DATABASE）：
//
//	MYSQL_TEST_DSN='root:pass@tcp(127.0.0.1:3306)/ai_go_mall_test?charset=utf8mb4&parseTime=true' \
//	  go test -tags=integration ./internal/service/mall/...
//
// 准备：
//   - env MYSQL_TEST_DSN 拿到 DSN，未设置 → skip；
//   - 用 bootstrap 连接 DROP + CREATE dbname；库名必须含 "test"（安全闸）；
//   - 重新用 dbname 连库跑 migrate.Up()；
//   - seed fixture（池内总库存刻意设为 5，便于快速售罄）。
//
// 期望（commission_rate = 10% 快照，price = 1000）：
//   - 抽满 5 单 → 池 total_stock = 0 → lifecycle: selling → sold_out（CAS）；
//   - 账单自动生成，且明细**恰好 5 行**（含售罄那一单，验证 P0 时序修正）；
//   - total = 5000, commission = 500, payout = 4500；
//   - 供应商 Confirm → confirmed；admin MarkPaid → paid；
//   - MarkPaid 后 supplier.balance = 5000（抽卡累计）- 4500（打款）= 500；
//   - 商品 lifecycle = settled。
//
// 包名刻意用 mall_test（外部测试包）而非 mall：本用例需要同时构造
// user.DrawService 与 supplier.BillService，而 service/supplier 反过来 import
// service/mall，写在包内会形成 import cycle。外部测试包天然无此问题，且强制
// 只走公开 API（NewBillService 等），更贴近真实调用方式。
package mall_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mysqldriver "github.com/go-sql-driver/mysql"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"

	"ai-go-mall/internal/domain/state"
	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/migrate"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	userRepo "ai-go-mall/internal/repository/user"
	mallSvc "ai-go-mall/internal/service/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
	"ai-go-mall/internal/service/user"

	migratelib "github.com/golang-migrate/migrate/v4"
)

// =============================================================================
// test wiring
// =============================================================================

// batchTestEnv 把「连真 MySQL + 跑 migrate + 注册 ctx DB + 拿 services」压成一个 struct。
type batchTestEnv struct {
	t     *testing.T
	gdb   *gorm.DB
	rawDB *sql.DB
	cfg   config.DatabaseConfig

	supplierID int64
	blindBoxID int64
	poolID     int64
	userID     int64
	cardID     int64

	drawSvc   user.DrawService
	billSvc   mallSvc.BillService
	supBillSvc supplierSvc.BillService
}

// batchRepoRoot 从当前工作目录向上找到含 go.mod 的目录。
//
// go test 的工作目录是**包目录**（internal/service/mall），所以任何仓库根相对
// 路径（如 cmd/migrate/migrations）都必须先经这里解析。
func batchRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// parseBatchDSN 把 DSN 解析成 config.DatabaseConfig。
//
// 与 admin 包的同名 helper 独立维护：本包是外部测试包，且不应跨包复用测试代码。
func parseBatchDSN(t *testing.T, dsn string) config.DatabaseConfig {
	t.Helper()
	cfg, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	out := config.DatabaseConfig{
		Type:   "mysql",
		Prefix: "",
		Write: config.DBInstanceConfig{
			Host:            stripAddrPort(cfg.Addr),
			Port:            portFromAddr(cfg.Addr),
			Username:        cfg.User,
			Password:        cfg.Passwd,
			DBName:          cfg.DBName,
			MaxOpenConns:    50,
			MaxIdleConns:    5,
			ConnMaxLifetime: 10 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}
	out.Read.Enabled = false
	if out.Write.DBName == "" {
		t.Fatalf("DSN missing dbname (after / in DSN)")
	}
	return out
}

func stripAddrPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[:idx]
	}
	return addr
}

func portFromAddr(addr string) int {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		var p int
		_, _ = fmt.Sscanf(addr[idx+1:], "%d", &p)
		return p
	}
	return 3306
}

// batchMakeDSN 用 cfg + dbnameOverride 拼 DSN。
func batchMakeDSN(cfg config.DatabaseConfig, dbnameOverride string) string {
	name := cfg.Write.DBName
	if dbnameOverride != "" {
		name = dbnameOverride
	}
	mc := mysqldriver.NewConfig()
	mc.User = cfg.Write.Username
	mc.Passwd = cfg.Write.Password
	mc.Net = "tcp"
	mc.Addr = fmt.Sprintf("%s:%d", cfg.Write.Host, cfg.Write.Port)
	mc.DBName = name
	mc.ParseTime = true
	mc.Loc = time.Local
	mc.Params = map[string]string{"charset": "utf8mb4"}
	return mc.FormatDSN()
}

// NewBatchTestEnv：读 DSN → reset DB → migrate up → seed fixture → 接线。
func NewBatchTestEnv(t *testing.T) *batchTestEnv {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set; skipping integration test")
	}

	cfg := parseBatchDSN(t, dsn)

	// 0. 安全闸：本测试随后会 DROP DATABASE。库名必须明显是测试库（含 "test"），
	//    否则拒绝运行 —— 防止误把开发库 DSN 传进来导致整库数据被清空。
	if !strings.Contains(strings.ToLower(cfg.Write.DBName), "test") {
		t.Fatalf("refusing to DROP database %q: integration test requires a dedicated test database (name must contain \"test\")",
			cfg.Write.DBName)
	}

	// 1. drop + create dbname。
	bootstrap, err := sql.Open("mysql", batchMakeDSN(cfg, ""))
	if err != nil {
		t.Fatalf("open bootstrap mysql: %v", err)
	}
	t.Cleanup(func() { _ = bootstrap.Close() })
	ctx, cancelFn := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelFn()
	if _, err := bootstrap.ExecContext(ctx,
		fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.Write.DBName)); err != nil {
		t.Fatalf("drop db %s: %v", cfg.Write.DBName, err)
	}
	if _, err := bootstrap.ExecContext(ctx,
		fmt.Sprintf("CREATE DATABASE `%s` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci", cfg.Write.DBName)); err != nil {
		t.Fatalf("create db %s: %v", cfg.Write.DBName, err)
	}

	// 2. config 注入（默认费率 10%，与 admin 集成测试同口径）。
	config.SetForTest(&config.Config{
		Database: cfg,
		Mall: config.MallConfig{
			DefaultCommissionRate: 0.10,
		},
	})

	// 3. migrate up。
	migrationsDir := filepath.Join(batchRepoRoot(t), "cmd", "migrate", "migrations")
	mig, err := migrate.New(cfg, migrationsDir)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	_ = mig.Close()

	// 4. 打开 GORM。
	gdb, err := gorm.Open(gormmysql.Open(batchMakeDSN(cfg, cfg.Write.DBName)), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			TablePrefix:   cfg.Prefix,
			SingularTable: true,
		},
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	rawDB, err := gdb.DB()
	if err != nil {
		t.Fatalf("gdb.DB: %v", err)
	}
	t.Cleanup(func() { _ = rawDB.Close() })

	// 5. seed fixture。
	seed := seedBatchFixture(t, gdb)

	env := &batchTestEnv{
		t:          t,
		gdb:        gdb,
		rawDB:      rawDB,
		cfg:        cfg,
		supplierID: seed.supplierID,
		blindBoxID: seed.blindBoxID,
		poolID:     seed.poolID,
		userID:     seed.userID,
		cardID:     seed.cardID,
	}

	// 6. 接线：billSvc 必须先建（drawSvc 依赖它），且两者共用同一组 repo。
	env.billSvc = mallSvc.NewBillService(mallSvc.BillServiceDeps{
		BillRepo:     mallRepo.NewSettlementBillRepository(),
		BoxRepo:      mallRepo.NewBlindBoxRepository(),
		SupplierRepo: supplierRepo.NewSupplierRepository(),
		OrderRepo:    mallRepo.NewDrawOrderRepository(),
	})
	env.drawSvc = user.NewDrawService(user.DrawServiceDeps{
		BlindBoxRepo:   mallRepo.NewBlindBoxRepository(),
		CardPoolRepo:   mallRepo.NewCardPoolRepository(),
		PromotionRepo:  mallRepo.NewPromotionRepository(),
		SupplierRepo:   supplierRepo.NewSupplierRepository(),
		UserRepo:       userRepo.NewRepository(),
		DrawOrderRepo:  mallRepo.NewDrawOrderRepository(),
		CardRepo:       mallRepo.NewCardRepository(),
		SettlementRepo: mallRepo.NewSettlementRepository(),
		BillSvc:        env.billSvc,
	})
	env.supBillSvc = supplierSvc.NewBillService(supplierSvc.BillServiceDeps{
		BillRepo: mallRepo.NewSettlementBillRepository(),
	})
	return env
}

// Ctx 构造一个 gin.Context，绑上 testEnv 的 *gorm.DB。
func (e *batchTestEnv) Ctx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/integration/batch-bill", nil)
	c.Set("ai_go_mall.db", e.gdb.WithContext(c.Request.Context()))
	return c
}

// =============================================================================
// Fixture
// =============================================================================

type batchSeedResult struct {
	supplierID int64
	blindBoxID int64
	poolID     int64
	userID     int64
	cardID     int64
}

// seedBatchFixture 插 1 供应商（commission_rate=NULL → 走 config 默认 10%）
// + 1 盲盒（active/on_sale，lifecycle 走列默认 selling）+ 1 卡池 +
// 1 张卡（weight=10000）+ 1 用户（balance 充足）。
//
// **关键**：池 item 的 stock 设为 5（而非 admin 用例的 1000），让「抽 5 单即售罄」
// 成为可控的短流程，从而能精确断言「售罄那一单在账单明细里」。
func seedBatchFixture(t *testing.T, db *gorm.DB) batchSeedResult {
	t.Helper()

	sup := mall.MallSupplier{
		Name:           "Batch Settlement Supplier",
		Status:         mall.StatusActive,
		IsFeatured:     true,
		Balance:        0,
		TotalSales:     0,
		CommissionRate: nil,
	}
	if err := db.Create(&sup).Error; err != nil {
		t.Fatalf("seed supplier: %v", err)
	}

	u := model.User{
		Username: "batch_integration_user",
		Password: "x",
		Balance:  10000,
		Status:   1,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	card := mall.MallCard{Name: "Batch Card"}
	if err := db.Create(&card).Error; err != nil {
		t.Fatalf("seed card: %v", err)
	}

	bb := mall.MallBlindBox{
		SupplierID: sup.ID,
		Name:       "Batch Blindbox",
		Price:      1000,
		Status:     mall.StatusActive,
		OnSale:     true,
		IsFeatured: false,
	}
	if err := db.Create(&bb).Error; err != nil {
		t.Fatalf("seed blindbox: %v", err)
	}

	pool := mall.MallCardPool{BlindBoxID: bb.ID}
	if err := db.Create(&pool).Error; err != nil {
		t.Fatalf("seed pool: %v", err)
	}

	// stock=5：抽满 5 单后 pool 总库存归零 → draw 事务尾部触发 FinalizeSoldOut。
	item := mall.MallCardPoolItem{
		PoolID: pool.ID,
		CardID: card.ID,
		Rarity: mall.RaritySSR,
		Weight: 10000,
		Stock:  5,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed pool item: %v", err)
	}

	return batchSeedResult{
		supplierID: sup.ID,
		blindBoxID: bb.ID,
		poolID:     pool.ID,
		userID:     u.ID,
		cardID:     card.ID,
	}
}

// =============================================================================
// Tests
// =============================================================================

// TestBatchBillIntegration_SoldOutToPaid 8.2 端到端。
func TestBatchBillIntegration_SoldOutToPaid(t *testing.T) {
	env := NewBatchTestEnv(t)
	ctx := env.Ctx()

	const draws = 5         // == 池内 stock，抽满即售罄
	const pricePerDraw = 1000.00

	// ---- 1. 连续抽卡至售罄 ----
	for i := 0; i < draws; i++ {
		if _, err := env.drawSvc.Draw(ctx, env.userID, env.blindBoxID); err != nil {
			t.Fatalf("Draw %d: %v", i+1, err)
		}
	}

	// ---- 2. 校验商品生命周期已由 draw 事务自动落定为 sold_out ----
	var box mall.MallBlindBox
	if err := env.gdb.First(&box, env.blindBoxID).Error; err != nil {
		t.Fatalf("read blindbox: %v", err)
	}
	if box.Lifecycle != mall.LifecycleSoldOut {
		t.Fatalf("after sold out, lifecycle = %q, want %q", box.Lifecycle, mall.LifecycleSoldOut)
	}

	// ---- 3. 校验账单已自动生成，且明细含全部 5 单（含售罄那一单）----
	//
	// 这是本用例的核心断言：draw 事务步骤 7.7 必须在订单 status='drawn' 落定
	// **之后**才调 FinalizeSoldOut，否则清空库存的那一单会漏出账单（P0 时序修正）。
	var bill mall.MallSettlementBill
	if err := env.gdb.Where("blind_box_id = ?", env.blindBoxID).First(&bill).Error; err != nil {
		t.Fatalf("read bill: %v", err)
	}

	if bill.Status != mall.BillStatusGenerated {
		t.Errorf("new bill status = %q, want %q", bill.Status, mall.BillStatusGenerated)
	}
	if bill.Version != 0 {
		t.Errorf("new bill version = %d, want 0", bill.Version)
	}
	if bill.CommissionRate != 0.10 {
		t.Errorf("bill.CommissionRate = %v, want 0.10", bill.CommissionRate)
	}

	const wantTotal = pricePerDraw * draws        // 5000
	const wantCommission = wantTotal * 0.10       // 500
	const wantPayout = wantTotal - wantCommission // 4500

	if bill.TotalAmount != wantTotal {
		t.Errorf("bill.TotalAmount = %v, want %v", bill.TotalAmount, wantTotal)
	}
	if bill.CommissionAmount != wantCommission {
		t.Errorf("bill.CommissionAmount = %v, want %v", bill.CommissionAmount, wantCommission)
	}
	if bill.PayoutAmount != wantPayout {
		t.Errorf("bill.PayoutAmount = %v, want %v", bill.PayoutAmount, wantPayout)
	}

	var items []mall.MallSettlementBillItem
	if err := env.gdb.Where("bill_id = ?", bill.ID).Find(&items).Error; err != nil {
		t.Fatalf("read bill items: %v", err)
	}
	if len(items) != draws {
		t.Fatalf("bill items = %d, want %d（售罄那一单必须被账单收录）", len(items), draws)
	}

	// 明细金额是 price / commission / payout 的逐单快照。
	for _, it := range items {
		if it.Amount != pricePerDraw {
			t.Errorf("item %d amount = %v, want %v", it.DrawOrderID, it.Amount, pricePerDraw)
		}
		if it.CommissionAmount != 100 {
			t.Errorf("item %d commission = %v, want 100", it.DrawOrderID, it.CommissionAmount)
		}
		if it.PayoutAmount != 900 {
			t.Errorf("item %d payout = %v, want 900", it.DrawOrderID, it.PayoutAmount)
		}
	}

	// 抽卡阶段 supplier.balance 已累计 5000（draw 事务内即时入账）。
	var sup mall.MallSupplier
	if err := env.gdb.First(&sup, env.supplierID).Error; err != nil {
		t.Fatalf("read supplier: %v", err)
	}
	if sup.Balance != wantTotal {
		t.Errorf("after draws supplier.balance = %v, want %v", sup.Balance, wantTotal)
	}

	// ---- 4. 供应商确认：generated → confirmed ----
	if err := env.supBillSvc.Confirm(ctx, env.supplierID, bill.ID); err != nil {
		t.Fatalf("supplier Confirm: %v", err)
	}
	if err := env.gdb.First(&bill, bill.ID).Error; err != nil {
		t.Fatalf("re-read bill: %v", err)
	}
	if bill.Status != mall.BillStatusConfirmed {
		t.Fatalf("after confirm status = %q, want %q", bill.Status, mall.BillStatusConfirmed)
	}

	// ---- 5. admin 打款：confirmed → paid + 扣余额 + 商品 settled ----
	if err := env.billSvc.MarkPaid(ctx, bill.ID, 1); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}

	if err := env.gdb.First(&bill, bill.ID).Error; err != nil {
		t.Fatalf("re-read bill after paid: %v", err)
	}
	if bill.Status != mall.BillStatusPaid {
		t.Errorf("after MarkPaid status = %q, want %q", bill.Status, mall.BillStatusPaid)
	}

	// 余额 = 抽卡累计 5000 - 打款 4500 = 500。
	const wantSupAfterPaid = wantTotal - wantPayout
	if err := env.gdb.First(&sup, env.supplierID).Error; err != nil {
		t.Fatalf("re-read supplier: %v", err)
	}
	if sup.Balance != wantSupAfterPaid {
		t.Errorf("after MarkPaid supplier.balance = %v, want %v", sup.Balance, wantSupAfterPaid)
	}

	// 商品生命周期落定 settled。
	if err := env.gdb.First(&box, env.blindBoxID).Error; err != nil {
		t.Fatalf("re-read blindbox: %v", err)
	}
	if box.Lifecycle != mall.LifecycleSettled {
		t.Errorf("after MarkPaid lifecycle = %q, want %q", box.Lifecycle, mall.LifecycleSettled)
	}
}

// TestBatchBillIntegration_RejectThenRegenerate 覆盖驳回 → 重核回 generated 分支。
//
// 验证 rejected 不是终态：admin 重核后 version+1 且金额被整体更新，
// 供应商可再次确认——这是「一次核对不上、改了金额再来」的真实业务路径。
func TestBatchBillIntegration_RejectThenRegenerate(t *testing.T) {
	env := NewBatchTestEnv(t)
	ctx := env.Ctx()

	const draws = 5
	for i := 0; i < draws; i++ {
		if _, err := env.drawSvc.Draw(ctx, env.userID, env.blindBoxID); err != nil {
			t.Fatalf("Draw %d: %v", i+1, err)
		}
	}

	var bill mall.MallSettlementBill
	if err := env.gdb.Where("blind_box_id = ?", env.blindBoxID).First(&bill).Error; err != nil {
		t.Fatalf("read bill: %v", err)
	}
	if bill.Status != mall.BillStatusGenerated {
		t.Fatalf("initial status = %q, want %q", bill.Status, mall.BillStatusGenerated)
	}

	// ---- 供应商驳回（必填原因）----
	if err := env.supBillSvc.Reject(ctx, env.supplierID, bill.ID, "金额核对不一致"); err != nil {
		t.Fatalf("supplier Reject: %v", err)
	}
	if err := env.gdb.First(&bill, bill.ID).Error; err != nil {
		t.Fatalf("re-read bill: %v", err)
	}
	if bill.Status != mall.BillStatusRejected {
		t.Fatalf("after reject status = %q, want %q", bill.Status, mall.BillStatusRejected)
	}

	// ---- admin 重核：rejected → generated，version+1，金额更新 ----
	const newTotal = 4800.00
	const newCommission = 480.00
	const newPayout = 4320.00
	if err := env.billSvc.Regenerate(ctx, bill.ID, mallRepo.BillRegenAmounts{
		TotalAmount:      newTotal,
		CommissionAmount: newCommission,
		PayoutAmount:     newPayout,
	}, "按协商价重核"); err != nil {
		t.Fatalf("Regenerate: %v", err)
	}

	if err := env.gdb.First(&bill, bill.ID).Error; err != nil {
		t.Fatalf("re-read bill after regenerate: %v", err)
	}
	if bill.Status != mall.BillStatusGenerated {
		t.Errorf("after regenerate status = %q, want %q", bill.Status, mall.BillStatusGenerated)
	}
	if bill.Version != 1 {
		t.Errorf("after regenerate version = %d, want 1", bill.Version)
	}
	if bill.TotalAmount != newTotal {
		t.Errorf("after regenerate total = %v, want %v", bill.TotalAmount, newTotal)
	}
	if bill.PayoutAmount != newPayout {
		t.Errorf("after regenerate payout = %v, want %v", bill.PayoutAmount, newPayout)
	}

	// ---- 重核后可再次确认（rejected 非终态）----
	if err := env.supBillSvc.Confirm(ctx, env.supplierID, bill.ID); err != nil {
		t.Fatalf("Confirm after regenerate: %v", err)
	}
	if err := env.gdb.First(&bill, bill.ID).Error; err != nil {
		t.Fatalf("re-read bill: %v", err)
	}
	if bill.Status != mall.BillStatusConfirmed {
		t.Errorf("after re-confirm status = %q, want %q", bill.Status, mall.BillStatusConfirmed)
	}
}

// TestBatchBillIntegration_IllegalTransitionBlocked 状态机防线。
//
// generated 直接打款（跳过供应商确认）必须被拒绝 —— 这是资金安全的关键约束：
// admin 不能在供应商未确认金额的情况下扣款。
func TestBatchBillIntegration_IllegalTransitionBlocked(t *testing.T) {
	env := NewBatchTestEnv(t)
	ctx := env.Ctx()

	const draws = 5
	for i := 0; i < draws; i++ {
		if _, err := env.drawSvc.Draw(ctx, env.userID, env.blindBoxID); err != nil {
			t.Fatalf("Draw %d: %v", i+1, err)
		}
	}

	var bill mall.MallSettlementBill
	if err := env.gdb.Where("blind_box_id = ?", env.blindBoxID).First(&bill).Error; err != nil {
		t.Fatalf("read bill: %v", err)
	}

	// generated 状态下直接 MarkPaid → 状态机拒绝。
	err := env.billSvc.MarkPaid(ctx, bill.ID, 1)
	if err == nil {
		t.Fatal("MarkPaid on generated bill should fail, got nil")
	}
	if !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("MarkPaid error = %v, want state.ErrInvalidStateTransition", err)
	}

	// 账单与余额均未被改动。
	if err := env.gdb.First(&bill, bill.ID).Error; err != nil {
		t.Fatalf("re-read bill: %v", err)
	}
	if bill.Status != mall.BillStatusGenerated {
		t.Errorf("bill status changed to %q, want still %q", bill.Status, mall.BillStatusGenerated)
	}

	var sup mall.MallSupplier
	if err := env.gdb.First(&sup, env.supplierID).Error; err != nil {
		t.Fatalf("read supplier: %v", err)
	}
	const wantBalance = 1000.00 * draws
	if sup.Balance != wantBalance {
		t.Errorf("supplier.balance = %v, want %v（打款被拒后不应变化）", sup.Balance, wantBalance)
	}

	// 商品也不应被推进到 settled。
	var box mall.MallBlindBox
	if err := env.gdb.First(&box, env.blindBoxID).Error; err != nil {
		t.Fatalf("read blindbox: %v", err)
	}
	if box.Lifecycle != mall.LifecycleSoldOut {
		t.Errorf("lifecycle = %q, want %q", box.Lifecycle, mall.LifecycleSoldOut)
	}
}
