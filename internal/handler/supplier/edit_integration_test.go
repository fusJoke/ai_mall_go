//go:build integration

// Package supplier —— 编辑接口的集成回归测试（真实 MySQL + 真 handler/service/repository）。
//
// 复现并锁定两个 500：
//
//	POST /supplier/products/edit   只传部分字段 → 零值整行写 → created_at='0000-00-00'
//	POST /supplier/promotions/edit 只传部分字段 → 同上（且未传价格被清零、状态被改写）
//
// 根因：handler 只把「客户端传了的字段」落到结构体上（其余是 Go 零值），
// 而仓储 Update 是 db.Save（全列写）。MySQL 严格模式拒绝零值 created_at → 500；
// 若 sql_mode 宽松，同一路径会静默把未传字段清空。
//
// 断言口径：HTTP 200 + **未传字段保持 DB 原值** + created_at 不被覆盖 +
// 编辑不会把已禁用的活动重新启用。
//
// 运行（必须用专用测试库，库名含 "test"；本测试会 DROP DATABASE）：
//
//	MYSQL_TEST_DSN='root:root123@tcp(127.0.0.1:3307)/ai_go_mall_test?charset=utf8mb4&parseTime=true&loc=Local' \
//	  go test -tags=integration ./internal/handler/supplier/ -run TestEditIntegration
package supplier

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	gosqlmysql "github.com/go-sql-driver/mysql"
	migratelib "github.com/golang-migrate/migrate/v4"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/migrate"
	"ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierSvc "ai-go-mall/internal/service/supplier"
)

// supplierCtxKey 必须与 middleware.supplierContextKey 一致（SupplierAuth 写入的键）。
const supplierCtxKey = "supplier.current"

type editEnv struct {
	t          *testing.T
	gdb        *gorm.DB
	supplierID int64
	boxID      int64
	promoID    int64
	productH   *ProductHandler
	promoH     *PromotionHandler
}

func newEditEnv(t *testing.T) *editEnv {
	t.Helper()

	dsn := os.Getenv("MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("MYSQL_TEST_DSN not set; skipping integration test")
	}
	cfg := editParseDSN(t, dsn)
	if !strings.Contains(strings.ToLower(cfg.Write.DBName), "test") {
		t.Fatalf("refusing to DROP database %q: integration test requires a dedicated test database (name must contain \"test\")",
			cfg.Write.DBName)
	}

	// 1) 干净库。
	bootstrap, err := sql.Open("mysql", editDSN(cfg, ""))
	if err != nil {
		t.Fatalf("open bootstrap: %v", err)
	}
	defer func() { _ = bootstrap.Close() }()
	if _, err := bootstrap.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", cfg.Write.DBName)); err != nil {
		t.Fatalf("drop db %s: %v", cfg.Write.DBName, err)
	}
	if _, err := bootstrap.Exec(fmt.Sprintf("CREATE DATABASE `%s` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_general_ci", cfg.Write.DBName)); err != nil {
		t.Fatalf("create db %s: %v", cfg.Write.DBName, err)
	}

	// 2) 配置注入 + 迁移到最新。
	config.SetForTest(&config.Config{Database: cfg})
	mig, err := migrate.New(cfg, filepath.Join(editRepoRoot(t), "cmd", "migrate", "migrations"))
	if err != nil {
		t.Fatalf("migrate.New: %v", err)
	}
	if err := mig.Up(); err != nil && !errors.Is(err, migratelib.ErrNoChange) {
		t.Fatalf("migrate up: %v", err)
	}
	_ = mig.Close()

	// 3) GORM 句柄（测试自行开连接；repository.DB(c) 从 gin.Context 读）。
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{DSN: editDSN(cfg, cfg.Write.DBName)}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm open: %v", err)
	}

	// 4) 夹具：供应商 + 盲盒 + 卡池 + 一条「已禁用」的活动。
	sup := mall.MallSupplier{Name: "IT Supplier", Status: mall.StatusActive}
	if err := gdb.Create(&sup).Error; err != nil {
		t.Fatalf("seed supplier: %v", err)
	}
	box := mall.MallBlindBox{
		SupplierID:  sup.ID,
		Name:        "IT Box",
		Cover:       "https://cdn.example.com/it/box.png",
		Price:       99,
		Status:      mall.StatusActive,
		OnSale:      true,
		IsFeatured:  true,
		Description: "keep me",
	}
	if err := gdb.Create(&box).Error; err != nil {
		t.Fatalf("seed box: %v", err)
	}
	if err := gdb.Create(&mall.MallCardPool{BlindBoxID: box.ID}).Error; err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	now := time.Now()
	promo := mall.MallPromotion{
		SupplierID:    sup.ID,
		BlindBoxID:    box.ID,
		OriginalPrice: 100,
		PromoPrice:    80,
		StartAt:       now,
		EndAt:         now.Add(24 * time.Hour),
		Status:        mall.StatusDisabled, // 已禁用：编辑不该把它重新启用
	}
	if err := gdb.Create(&promo).Error; err != nil {
		t.Fatalf("seed promo: %v", err)
	}

	// 5) 真实 service + handler（Cache/TxRunner 留 nil，走默认）。
	productSvc := supplierSvc.NewProductService(supplierSvc.ProductServiceDeps{
		BlindBoxRepo: mallRepo.NewBlindBoxRepository(),
		CardPoolRepo: mallRepo.NewCardPoolRepository(),
	})
	promoSvc := supplierSvc.NewPromotionService(supplierSvc.PromotionServiceDeps{
		PromotionRepo: mallRepo.NewPromotionRepository(),
		BlindBoxRepo:  mallRepo.NewBlindBoxRepository(),
	})

	return &editEnv{
		t:          t,
		gdb:        gdb,
		supplierID: sup.ID,
		boxID:      box.ID,
		promoID:    promo.ID,
		productH:   NewProductHandler(productSvc),
		promoH:     NewPromotionHandler(promoSvc),
	}
}

// call 走真实 handler：注入请求作用域 DB + 供应商身份，返回响应记录器。
func (e *editEnv) call(h gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, "/supplier/edit", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(database.CtxKey, e.gdb.WithContext(c.Request.Context()))
	c.Set(supplierCtxKey, &mall.MallSupplierUser{SupplierID: e.supplierID, Status: 1})
	h(c)
	return w
}

// TestEditIntegration_ProductPartialEditKeepsOmittedFields 复现 C1：
// 只传 {id, price} 编辑商品，未传字段必须保持 DB 原值。
func TestEditIntegration_ProductPartialEditKeepsOmittedFields(t *testing.T) {
	env := newEditEnv(t)

	w := env.call(env.productH.Edit, fmt.Sprintf(`{"id":%d,"price":9.9}`, env.boxID))
	if w.Code != http.StatusOK {
		t.Fatalf("products/edit status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got mall.MallBlindBox
	if err := env.gdb.First(&got, env.boxID).Error; err != nil {
		t.Fatalf("reload box: %v", err)
	}
	if got.Price != 9.9 {
		t.Errorf("price = %v, want 9.9（本次确实要改的字段没生效）", got.Price)
	}
	if got.Name != "IT Box" {
		t.Errorf("name = %q, want %q（未传字段被零值覆盖）", got.Name, "IT Box")
	}
	if got.Cover != "https://cdn.example.com/it/box.png" {
		t.Errorf("cover = %q, want 原值（未传字段被零值覆盖）", got.Cover)
	}
	if got.Description != "keep me" {
		t.Errorf("description = %q, want %q（未传字段被零值覆盖）", got.Description, "keep me")
	}
	if got.Status != mall.StatusActive {
		t.Errorf("status = %q, want %q（未传字段被零值覆盖）", got.Status, mall.StatusActive)
	}
	if !got.OnSale {
		t.Errorf("on_sale = false, want true（未传字段被零值覆盖，商品会从 C 端消失）")
	}
	if !got.IsFeatured {
		t.Errorf("is_featured = false, want true（未传字段被零值覆盖）")
	}
	if got.SupplierID != env.supplierID {
		t.Errorf("supplier_id = %d, want %d", got.SupplierID, env.supplierID)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("created_at 被写成零值（db.Save 全列写会把 created_at 一起覆盖）")
	}
}

// TestEditIntegration_PromotionPartialEditKeepsWindowAndStatus 复现 C2：
// 只传 {id, promo_price} 编辑活动，未传价格/周期/状态必须保持原值。
func TestEditIntegration_PromotionPartialEditKeepsWindowAndStatus(t *testing.T) {
	env := newEditEnv(t)

	var before mall.MallPromotion
	if err := env.gdb.First(&before, env.promoID).Error; err != nil {
		t.Fatalf("reload promo before: %v", err)
	}

	// promo_price=88 < original_price=100（DB 原值）→ 合法。
	w := env.call(env.promoH.Edit, fmt.Sprintf(`{"id":%d,"promo_price":88}`, env.promoID))
	if w.Code != http.StatusOK {
		t.Fatalf("promotions/edit status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got mall.MallPromotion
	if err := env.gdb.First(&got, env.promoID).Error; err != nil {
		t.Fatalf("reload promo: %v", err)
	}
	if got.PromoPrice != 88 {
		t.Errorf("promo_price = %v, want 88（本次确实要改的字段没生效）", got.PromoPrice)
	}
	if got.OriginalPrice != 100 {
		t.Errorf("original_price = %v, want 100（未传字段被零值覆盖）", got.OriginalPrice)
	}
	if got.Status != mall.StatusDisabled {
		t.Errorf("status = %q, want %q（编辑把已禁用的活动重新启用了）", got.Status, mall.StatusDisabled)
	}
	if !got.StartAt.Equal(before.StartAt) || !got.EndAt.Equal(before.EndAt) {
		t.Errorf("活动周期被改动: %v~%v → %v~%v", before.StartAt, before.EndAt, got.StartAt, got.EndAt)
	}
	if got.CreatedAt.IsZero() {
		t.Errorf("created_at 被写成零值（db.Save 全列写会把 created_at 一起覆盖）")
	}
}

// =============================================================================
// DSN + 路径helpers（与 service/admin 的集成测试同构；跨包无法复用）
// =============================================================================

func editParseDSN(t *testing.T, dsn string) config.DatabaseConfig {
	t.Helper()
	cfg, err := gosqlmysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	if cfg.DBName == "" {
		t.Fatalf("DSN missing dbname (after / in DSN)")
	}
	out := config.DatabaseConfig{
		Type: "mysql",
		Write: config.DBInstanceConfig{
			Host:            editStripPort(cfg.Addr),
			Port:            editPort(cfg.Addr),
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
	return out
}

func editStripPort(addr string) string {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		return addr[:idx]
	}
	return addr
}

func editPort(addr string) int {
	if idx := strings.LastIndex(addr, ":"); idx >= 0 {
		var p int
		_, _ = fmt.Sscanf(addr[idx+1:], "%d", &p)
		return p
	}
	return 3306
}

func editDSN(cfg config.DatabaseConfig, dbnameOverride string) string {
	name := cfg.Write.DBName
	if dbnameOverride != "" {
		name = dbnameOverride
	}
	mc := gosqlmysql.NewConfig() // NewConfig 带默认值（AllowNativePasswords=true）
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

// editRepoRoot 向上找到含 go.mod 的目录（go test 的 cwd 是包目录）。
func editRepoRoot(t *testing.T) string {
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
