//go:build integration

// Package admin 集成测试（task 8.13 / 8.14）。
//
// 跑通端到端结算流：10 单抽卡 → admin 生成 09 月结算单 → preview →
// generate → 校验数字 → mark_paid → 校验 supplier.balance + platform_ledger
// 三方一致；并验证 mark_paid 二次调用返回 ErrSettlementAlreadyPaid。
//
// 必须挂 //go:build integration build tag，默认 `go test ./...` 不跑。
// 运行方式（**必须**用专用测试库，库名含 "test"；测试会 DROP DATABASE）：
//
//	MYSQL_TEST_DSN='root:pass@tcp(127.0.0.1:3306)/ai_go_mall_test?charset=utf8mb4&parseTime=true' \
//	  go test -tags=integration ./internal/service/admin/...
//
// 准备：
//   - env MYSQL_TEST_DSN 拿到 DSN，未设置 → skip；
//   - 用 bootstrap 连接 DROP + CREATE dbname；
//   - 重新用 dbname 连库跑 migrate.Up()；
//   - seed fixture 后跑 Draw ×10 + settlement 流。
//
// 期望（commission_rate 10% 默认）：
//   - 抽完 10 单后 supplier.balance = 10000，ledger.total_revenue = 10000；
//   - preview/generate：total=10000, commission=1000, payout=9000；
//   - generate 不动余额；
//   - mark_paid 后 supplier.balance = 10000 - 9000 = 1000；
//   - 第二次 mark_paid → ErrSettlementAlreadyPaid。
package admin

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
	_ "github.com/go-sql-driver/mysql"
	gormmysql "github.com/go-sql-driver/mysql"
	migratelib "github.com/golang-migrate/migrate/v4"
	"gorm.io/driver/mysql"
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
	"ai-go-mall/internal/service/user"
)

// =============================================================================
// test wiring
// =============================================================================

// testEnv 把"连真 MySQL + 跑 migrate + 注册 ctx DB + 拿 services"压成一个 struct。
type testEnv struct {
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
	settleSvc SettlementService

	periodStart time.Time
	periodEnd   time.Time
}

// NewTestEnv：读 DSN → reset DB → migrate up → seed fixture → 接线。
// repoRoot 从当前工作目录向上找到含 go.mod 的目录。
//
// go test 的工作目录是**包目录**（internal/service/admin），所以测试里任何
// 仓库根相对路径（如 cmd/migrate/migrations）都必须先经这里解析。
func repoRoot(t *testing.T) string {
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

func NewTestEnv(t *testing.T) *testEnv {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set; skipping integration test")
	}

	cfg := parseDSN(t, dsn)

	// 0. 安全闸：本测试随后会 DROP DATABASE。库名必须明显是测试库（含 "test"），
	//    否则拒绝运行 —— 曾把开发库 DSN 传进来，整库数据被清空。
	if !strings.Contains(strings.ToLower(cfg.Write.DBName), "test") {
		t.Fatalf("refusing to DROP database %q: integration test requires a dedicated test database (name must contain \"test\")",
			cfg.Write.DBName)
	}

	// 1. drop + create dbname。
	bootstrap, err := sql.Open("mysql", makeDSN(cfg, ""))
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

	// 2. config 注入。
	config.SetForTest(&config.Config{
		Database: cfg,
		Mall: config.MallConfig{
			DefaultCommissionRate: 0.10,
		},
	})

	// 3. migrate up（直接用 internal/infra/migrate）。
	// 路径必须相对**仓库根**解析：`go test` 的工作目录是包目录
	// （internal/service/admin），写成 "cmd/migrate/migrations" 会 GetFileAttributesEx
	// 失败 —— 8.13/8.14 因此一直跑不起来。
	migrationsDir := filepath.Join(repoRoot(t), "cmd", "migrate", "migrations")
	mig, err := migrate.New(cfg, migrationsDir)
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	_ = mig.Close()

	// 4. 打开 GORM。
	gdb, err := gorm.Open(mysql.Open(makeDSN(cfg, cfg.Write.DBName)), &gorm.Config{
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

	// 5. seed fixture。结算周期 = **当月**窗口 [本月 1 号, 下月 1 号)。
	//
	// 周期必须包含抽卡时刻：下面 Draw 产生的订单 created_at = now，而
	// ListEligibleOrders 的条件是 created_at ∈ [periodStart, periodEnd)。
	// 原实现取「本月 1 号减 30 天」得到的窗口在本月 1 号就结束了，抽卡发生在
	// 之后 → 恒为 0 条可结算 → preview.OrderCount = 0 且 Generate 报
	// "no orders to settle"（8.13/8.14 因此从未真正通过过）。
	now := time.Now()
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
	periodEnd := periodStart.AddDate(0, 1, 0)

	seed := seedFixture(t, gdb)

	env := &testEnv{
		t:           t,
		gdb:         gdb,
		rawDB:       rawDB,
		cfg:         cfg,
		supplierID:  seed.supplierID,
		blindBoxID:  seed.blindBoxID,
		poolID:      seed.poolID,
		userID:      seed.userID,
		cardID:      seed.cardID,
		periodStart: periodStart,
		periodEnd:   periodEnd,
	}
	env.drawSvc = user.NewDrawService(user.DrawServiceDeps{
		BlindBoxRepo:   mallRepo.NewBlindBoxRepository(),
		CardPoolRepo:   mallRepo.NewCardPoolRepository(),
		PromotionRepo:  mallRepo.NewPromotionRepository(),
		SupplierRepo:   supplierRepo.NewSupplierRepository(),
		UserRepo:       userRepo.NewRepository(),
		DrawOrderRepo:  mallRepo.NewDrawOrderRepository(),
		CardRepo:       mallRepo.NewCardRepository(),
		SettlementRepo: mallRepo.NewSettlementRepository(),
	})
	env.settleSvc = NewSettlementService(SettlementServiceDeps{
		SettlementRepo: mallRepo.NewSettlementRepository(),
		SupplierRepo:   supplierRepo.NewSupplierRepository(),
		DrawOrderRepo:  mallRepo.NewDrawOrderRepository(),
	})
	return env
}

// Ctx 构造一个 gin.Context，绑上 testEnv 的 *gorm.DB。
func (e *testEnv) Ctx() *gin.Context {
	gin.SetMode(gin.TestMode)
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/integration/test", nil)
	c.Set("ai_go_mall.db", e.gdb.WithContext(c.Request.Context()))
	return c
}

// =============================================================================
// Tests
// =============================================================================

// TestSettlementIntegration_FullFlow 8.13 端到端。
func TestSettlementIntegration_FullFlow(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Ctx()

	// 1. 抽 10 单。
	const draws = 10
	const pricePerDraw = 1000.00
	for i := 0; i < draws; i++ {
		if _, err := env.drawSvc.Draw(ctx, env.userID, env.blindBoxID); err != nil {
			t.Fatalf("Draw %d: %v", i+1, err)
		}
	}

	// 中间断言：抽完 10 单后 supplier.balance = 10000，user.balance = 0。
	var sup mall.MallSupplier
	if err := env.gdb.First(&sup, env.supplierID).Error; err != nil {
		t.Fatalf("read supplier: %v", err)
	}
	wantSupAfterDraws := pricePerDraw * float64(draws)
	if sup.Balance != wantSupAfterDraws {
		t.Errorf("after draws supplier.balance = %v, want %v", sup.Balance, wantSupAfterDraws)
	}
	var u model.User
	if err := env.gdb.First(&u, env.userID).Error; err != nil {
		t.Fatalf("read user: %v", err)
	}
	if u.Balance != 0 {
		t.Errorf("after draws user.balance = %v, want 0", u.Balance)
	}

	// 抽卡 → ledger total_revenue 累计到 10000。
	var revCounter mall.MallPlatformLedger
	if err := env.gdb.Where("counter_key = ?", "total_revenue").First(&revCounter).Error; err != nil {
		t.Fatalf("read total_revenue: %v", err)
	}
	if revCounter.Amount != wantSupAfterDraws {
		t.Errorf("ledger.total_revenue = %v, want %v", revCounter.Amount, wantSupAfterDraws)
	}

	// 2. admin preview。
	preview, err := env.settleSvc.Preview(ctx, env.supplierID, env.periodStart, env.periodEnd)
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if preview.OrderCount != draws {
		t.Errorf("preview.OrderCount = %d, want %d", preview.OrderCount, draws)
	}
	if preview.CommissionRate != 0.10 {
		t.Errorf("preview.CommissionRate = %v, want 0.10", preview.CommissionRate)
	}
	const wantTotal = pricePerDraw * draws        // 10000
	const wantCommission = wantTotal * 0.10       // 1000
	const wantPayout = wantTotal - wantCommission // 9000
	if preview.TotalAmount != wantTotal {
		t.Errorf("preview.TotalAmount = %v, want %v", preview.TotalAmount, wantTotal)
	}
	if preview.CommissionAmount != wantCommission {
		t.Errorf("preview.CommissionAmount = %v, want %v", preview.CommissionAmount, wantCommission)
	}
	if preview.PayoutAmount != wantPayout {
		t.Errorf("preview.PayoutAmount = %v, want %v", preview.PayoutAmount, wantPayout)
	}

	// 3. admin generate。
	settlement, err := env.settleSvc.Generate(ctx, env.supplierID, env.periodStart, env.periodEnd)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if settlement.ID == 0 {
		t.Fatalf("Generate returned settlement with ID=0")
	}
	if settlement.Status != mall.SettlementStatusProcessing {
		// spec 14.6: Generate 完成后状态机立刻推进 pending → processing，
		// service 返回的 settlement.Status 也同步更新为 processing。
		t.Errorf("settlement.Status = %v, want processing", settlement.Status)
	}

	// 4. generate 不动 supplier.balance；ledger 不动。
	if err := env.gdb.First(&sup, env.supplierID).Error; err != nil {
		t.Fatalf("read supplier after generate: %v", err)
	}
	if sup.Balance != wantSupAfterDraws {
		t.Errorf("after generate supplier.balance = %v, want %v (unchanged)",
			sup.Balance, wantSupAfterDraws)
	}
	if err := env.gdb.Where("counter_key = ?", "total_revenue").First(&revCounter).Error; err != nil {
		t.Fatalf("read total_revenue after generate: %v", err)
	}
	if revCounter.Amount != wantSupAfterDraws {
		t.Errorf("after generate ledger.total_revenue = %v, want %v (unchanged)",
			revCounter.Amount, wantSupAfterDraws)
	}

	// 5. admin mark_paid。
	const adminID = 42
	if err := env.settleSvc.MarkPaid(ctx, settlement.ID, adminID); err != nil {
		t.Fatalf("MarkPaid: %v", err)
	}

	// 6. mark_paid 后 supplier.balance = 10000 - 9000 = 1000。
	if err := env.gdb.First(&sup, env.supplierID).Error; err != nil {
		t.Fatalf("read supplier after mark_paid: %v", err)
	}
	wantSupFinal := wantSupAfterDraws - wantPayout
	if sup.Balance != wantSupFinal {
		t.Errorf("after mark_paid supplier.balance = %v, want %v", sup.Balance, wantSupFinal)
	}

	// settlement 状态置 paid + paid_at + paid_by。
	var paidSet mall.MallSettlement
	if err := env.gdb.First(&paidSet, settlement.ID).Error; err != nil {
		t.Fatalf("read settlement: %v", err)
	}
	if paidSet.Status != mall.SettlementStatusPaid {
		t.Errorf("settlement.Status = %v, want paid", paidSet.Status)
	}
	if paidSet.PaidAt == nil {
		t.Errorf("settlement.PaidAt is nil")
	}
	if paidSet.PaidBy == nil || *paidSet.PaidBy != adminID {
		t.Errorf("settlement.PaidBy = %v, want %d", paidSet.PaidBy, adminID)
	}

	// items 数 = draws。
	var itemCount int64
	if err := env.gdb.Model(&mall.MallSettlementItem{}).
		Where("settlement_id = ?", settlement.ID).Count(&itemCount).Error; err != nil {
		t.Fatalf("count items: %v", err)
	}
	if itemCount != draws {
		t.Errorf("settlement items count = %d, want %d", itemCount, draws)
	}

	// platform_ledger.total_revenue 不变（commission 留在平台）。
	if err := env.gdb.Where("counter_key = ?", "total_revenue").First(&revCounter).Error; err != nil {
		t.Fatalf("read total_revenue after mark_paid: %v", err)
	}
	if revCounter.Amount != wantSupAfterDraws {
		t.Errorf("after mark_paid ledger.total_revenue = %v, want %v",
			revCounter.Amount, wantSupAfterDraws)
	}
}

// TestSettlementIntegration_MarkPaidIdempotent 8.14 幂等。
func TestSettlementIntegration_MarkPaidIdempotent(t *testing.T) {
	env := NewTestEnv(t)
	ctx := env.Ctx()

	// 1. 抽 5 单 + generate。
	for i := 0; i < 5; i++ {
		if _, err := env.drawSvc.Draw(ctx, env.userID, env.blindBoxID); err != nil {
			t.Fatalf("Draw %d: %v", i+1, err)
		}
	}
	settlement, err := env.settleSvc.Generate(ctx, env.supplierID, env.periodStart, env.periodEnd)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// 2. 第一次 mark_paid → 成功。
	if err := env.settleSvc.MarkPaid(ctx, settlement.ID, 1); err != nil {
		t.Fatalf("first MarkPaid: %v", err)
	}

	// 3. 第二次 mark_paid → ErrInvalidStateTransition（spec 14：状态机
	//   paid → paid 视为非法转换，在调 repo.MarkPaid 之前 fail-fast）。
	//   历史行为返回 ErrSettlementAlreadyPaid，由 repo CAS UPDATE RowsAffected=0 触发；
	//   状态机接管后错误名变更，handler 仍映射 HTTP 409。
	if err := env.settleSvc.MarkPaid(ctx, settlement.ID, 1); !errors.Is(err, state.ErrInvalidStateTransition) {
		t.Errorf("second MarkPaid err = %v, want state.ErrInvalidStateTransition", err)
	}
}

// =============================================================================
// DSN parsing + helpers
// =============================================================================

// parseDSN 把 MYSQL_TEST_DSN 解析成 config.DatabaseConfig。
func parseDSN(t *testing.T, dsn string) config.DatabaseConfig {
	t.Helper()
	cfg, err := gormmysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	out := config.DatabaseConfig{
		Type:   "mysql",
		Prefix: "",
		Write: config.DBInstanceConfig{
			Host:            stripPortHost(cfg.Addr),
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

func stripPortHost(addr string) string {
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

// makeDSN 用 cfg + dbnameOverride 拼 DSN。
func makeDSN(cfg config.DatabaseConfig, dbnameOverride string) string {
	name := cfg.Write.DBName
	if dbnameOverride != "" {
		name = dbnameOverride
	}
	mc := gormmysql.NewConfig()
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

// =============================================================================
// Fixture seeding
// =============================================================================

type seedResult struct {
	supplierID int64
	blindBoxID int64
	poolID     int64
	userID     int64
	cardID     int64
}

// seedFixture 插 1 个供应商（commission_rate=NULL）+ 1 个盲盒 + 1 个卡池 +
// 1 张卡（pool item weight=10000，stock=1000）+ 1 个用户（balance=10000）。
//
// draw.go 抽卡事务步骤 8 用 INSERT ... NOW() 写订单 created_at，
// 期间 [periodStart..periodEnd) 由 NewTestEnv 在调用 seedFixture 前算好。
func seedFixture(t *testing.T, db *gorm.DB) seedResult {
	t.Helper()

	sup := mall.MallSupplier{
		Name:           "Acme Integration",
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
		Username: "integration_user",
		Password: "x",
		Balance:  10000,
		Status:   1,
	}
	if err := db.Create(&u).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}

	card := mall.MallCard{Name: "Integration Card"}
	if err := db.Create(&card).Error; err != nil {
		t.Fatalf("seed card: %v", err)
	}

	bb := mall.MallBlindBox{
		SupplierID: sup.ID,
		Name:       "Integration Blindbox",
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

	item := mall.MallCardPoolItem{
		PoolID: pool.ID,
		CardID: card.ID,
		Rarity: mall.RaritySSR,
		Weight: 10000,
		Stock:  1000,
	}
	if err := db.Create(&item).Error; err != nil {
		t.Fatalf("seed pool item: %v", err)
	}

	return seedResult{
		supplierID: sup.ID,
		blindBoxID: bb.ID,
		poolID:     pool.ID,
		userID:     u.ID,
		cardID:     card.ID,
	}
}
