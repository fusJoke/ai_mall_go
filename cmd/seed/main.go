// Command seed 一次性插入开发 / CI 用的种子数据：
//
//   - 1 个管理员账号（root / Passw0rd!）
//   - 3 个 mall 供应商（含 2 个 featured；commission_rate 三种情形各一个）
//   - 3 个 supplier_users（每个供应商 1 个登录账号；spec 规定 supplier_id 全局唯一）
//   - 30 张卡牌（覆盖 4 个稀有度）
//   - 6 个盲盒（每个供应商 2 个）
//   - 6 个卡池（1:1 与盲盒绑定）
//   - 每个卡池的 pool_items（rarity / weight / stock 分配）
//   - 2 个 active 限时特价活动
//   - 1 个 active 秒杀活动（total_stock=100, per_user_limit=1；用于前端秒杀页联调）
//   - 若干历史已抽卡订单（spec 8.8：用于 admin 结算流程联调）
//
// 用法：go run ./cmd/seed
//
// 设计：
//   - 所有写入走"按 unique key 反查 → 有就更新、无则插入"幂等模式，
//     重复运行不会产生重复数据。
//   - 密码统一用 bcrypt.DefaultCost 哈希；不打印明文密码。
//   - 金额 / 库存用「业务可读」的硬编码值（如 99.00、5000），
//     不引用外部配置文件。
//   - commission_rate 三种情形（spec 8.8）：
//     * sup1 (Kayou) NULL → 走 config.mall.default_commission_rate（10%）
//     * sup2 (Panini) 显式 0.08（8%，VIP 折扣）
//     * sup3 (Topps) NULL → 同 sup1 也走 10%
//     留两个 NULL 让结算流程 / 单测能验证「NULL → config 默认」语义。
//
// 不是生产脚本，只是开发辅助。
package main

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
)

const (
	adminUsername = "root"
	adminPassword = "Passw0rd!"
	adminNickname = "超级管理员"

	supplierPassword = "Passw0rd!"

	// 卡池权重：同池加和固定 10000
	weightSSR = 300  // 3%
	weightSR  = 1200 // 12%
	weightR   = 3500 // 35%
	weightN   = 5000 // 50%
)

func main() {
	if err := config.Init("."); err != nil {
		log.Fatalf("init config: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("init database: %v", err)
	}
	defer database.Reset()

	db := database.Get()

	// 强制 mall 子包 init() 执行（model 包 init 通常在加载期已执行，
	// 但显式 import 一下兜底，避免被 go vet 误判未使用）。
	_ = mall.MallCard{}

	now := time.Now()

	// 1. 管理员
	admID := seedAdmin(db, now)

	// 2. 供应商主体（3 个）
	suppliers := seedSuppliers(db, now)

	// 3. 供应商登录账号（每个供应商 2 个，共 6 个）
	supplierUsers := seedSupplierUsers(db, now, suppliers)

	// 4. 卡牌（30 张）
	cards := seedCards(db, now)

	// 5. 盲盒（每个供应商 2 个，共 6 个）
	blindBoxes := seedBlindBoxes(db, now, suppliers)

	// 6. 卡池（每个盲盒 1 个）
	pools := seedCardPools(db, now, blindBoxes)

	// 7. 卡池条目（rarity / weight / stock）
	seedCardPoolItems(db, now, pools, cards)

	// 8. 限时特价活动（2 个 active）
	seedPromotions(db, now, blindBoxes)

	// 9. 秒杀活动（1 个 active）—— 给前端秒杀页 / supplier 管理页做 fixture
	seedSeckillActivities(db, now, blindBoxes)

	// 10. 历史 drawn 订单（spec 8.8：给 admin 结算流程联调做 fixture）
	seedHistoricalDrawnOrders(db, now, blindBoxes)

	fmt.Printf("seed completed: admin_id=%d suppliers=%d supplier_users=%d cards=%d blind_boxes=%d pools=%d\n",
		admID, len(suppliers), len(supplierUsers), len(cards), len(blindBoxes), len(pools))
}

// =============================================================================
// 1. 管理员
// =============================================================================

func seedAdmin(db *gorm.DB, now time.Time) int64 {
	hash, err := bcrypt.GenerateFromPassword([]byte(adminPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash admin password: %v", err)
	}

	var existing model.Admin
	err = db.Where("username = ?", adminUsername).First(&existing).Error
	switch {
	case err == nil:
		existing.Password = string(hash)
		existing.Status = 1
		existing.LoginFailure = 0
		existing.UpdatedAt = now
		if err := db.Save(&existing).Error; err != nil {
			log.Fatalf("update admin: %v", err)
		}
		fmt.Printf("updated admin id=%d username=%s\n", existing.ID, existing.Username)
		return existing.ID
	default:
		adm := model.Admin{
			Username: adminUsername,
			Password: string(hash),
			Nickname: adminNickname,
			Status:   1,
		}
		adm.CreatedAt = now
		adm.UpdatedAt = now
		if err := db.Create(&adm).Error; err != nil {
			log.Fatalf("create admin: %v", err)
		}
		fmt.Printf("created admin id=%d username=%s\n", adm.ID, adm.Username)
		return adm.ID
	}
}

// =============================================================================
// 2. 供应商主体（3 个）
// =============================================================================

type supplierSeed struct {
	Name          string
	Logo          string
	Bio           string
	IsFeatured    bool
	ContactPhone  string
	CommissionRate *float64 // nil 走 config 默认
}

// supplierSeeds 静态种子：3 个供应商，2 个 featured。
//
// commission_rate 设计（spec 8.8）：
//   - sup1 (Kayou)  NULL → 走 config.mall.default_commission_rate (10%)
//   - sup2 (Panini) 显式 0.08（8%，VIP 折扣）
//   - sup3 (Topps)  NULL → 同 sup1 也走 10%
//
// 留两个 NULL 让结算流程 / 单测能验证「NULL → config 默认」语义。
var supplierSeeds = []supplierSeed{
	{
		Name: "Kayou 卡游旗舰店", Logo: "https://cdn.example.com/kayou/logo.png",
		Bio:          "国内卡牌龙头，覆盖动画 / 体育 / 游戏全品类。",
		IsFeatured:   true,
		ContactPhone: "13800000001",
		CommissionRate: nil, // 走 config 默认
	},
	{
		Name: "Panini 旗舰店", Logo: "https://cdn.example.com/panini/logo.png",
		Bio:          "意大利老牌球星卡品牌，NBA / FIFA 官方合作。",
		IsFeatured:   true,
		ContactPhone: "13800000002",
		CommissionRate: func() *float64 { v := 0.0800; return &v }(),
	},
	{
		Name: "Topps 旗舰店", Logo: "https://cdn.example.com/topps/logo.png",
		Bio:          "MLB / 英超官方球星卡供应商。",
		IsFeatured:   false,
		ContactPhone: "13800000003",
		CommissionRate: nil, // 走 config 默认
	},
}

func seedSuppliers(db *gorm.DB, now time.Time) []mall.MallSupplier {
	out := make([]mall.MallSupplier, 0, len(supplierSeeds))
	for _, s := range supplierSeeds {
		var existing mall.MallSupplier
		err := db.Where("name = ?", s.Name).First(&existing).Error
		switch {
		case err == nil:
			existing.Logo = s.Logo
			existing.Bio = s.Bio
			existing.Status = mall.StatusActive
			existing.IsFeatured = s.IsFeatured
			existing.ContactPhone = s.ContactPhone
			existing.CommissionRate = s.CommissionRate
			existing.UpdatedAt = now
			if err := db.Save(&existing).Error; err != nil {
				log.Fatalf("update supplier %q: %v", s.Name, err)
			}
			out = append(out, existing)
		default:
			sup := mall.MallSupplier{
				Name:           s.Name,
				Logo:           s.Logo,
				Bio:            s.Bio,
				Status:         mall.StatusActive,
				IsFeatured:     s.IsFeatured,
				ContactPhone:   s.ContactPhone,
				Balance:        0,
				TotalSales:     0,
				CommissionRate: s.CommissionRate,
			}
			sup.CreatedAt = now
			sup.UpdatedAt = now
			if err := db.Create(&sup).Error; err != nil {
				log.Fatalf("create supplier %q: %v", s.Name, err)
			}
			out = append(out, sup)
		}
	}
	fmt.Printf("seeded suppliers: %d\n", len(out))
	return out
}

// =============================================================================
// 3. 供应商登录账号（每个供应商 2 个）
// =============================================================================

func seedSupplierUsers(db *gorm.DB, now time.Time, suppliers []mall.MallSupplier) []mall.MallSupplierUser {
	hash, err := bcrypt.GenerateFromPassword([]byte(supplierPassword), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("hash supplier password: %v", err)
	}
	hashStr := string(hash)

	out := make([]mall.MallSupplierUser, 0)
	for _, sup := range suppliers {
		// 每个供应商 1 个账号：spec「mall_supplier_users field schema」规定
		// supplier_id 是全局唯一索引（一个供应商一个登录账号），迁移 000007 亦
		// 据此建了 UNIQUE KEY idx_mall_supplier_users_supplier；建第二个账号会
		// 撞 Error 1062。故这里只种 owner 一个账号。
		usernames := []string{
			sup.Name + "_owner",
		}
		for _, u := range usernames {
			var existing mall.MallSupplierUser
			err := db.Where("username = ?", u).First(&existing).Error
			switch {
			case err == nil:
				existing.Password = hashStr
				existing.Status = 1
				existing.UpdatedAt = now
				if err := db.Save(&existing).Error; err != nil {
					log.Fatalf("update supplier_user %q: %v", u, err)
				}
				out = append(out, existing)
			default:
				su := mall.MallSupplierUser{
					SupplierID: sup.ID,
					Username:   u,
					Password:   hashStr,
					Status:     1,
				}
				su.CreatedAt = now
				su.UpdatedAt = now
				if err := db.Create(&su).Error; err != nil {
					log.Fatalf("create supplier_user %q: %v", u, err)
				}
				out = append(out, su)
			}
		}
	}
	fmt.Printf("seeded supplier_users: %d\n", len(out))
	return out
}

// =============================================================================
// 4. 卡牌（30 张）
// =============================================================================

type cardSeed struct {
	Name   string
	Team   string
	Player string
	Rarity mall.MallCardRarity
}

// cardSeeds 静态种子：30 张卡，覆盖 4 个稀有度 + 多球队 / 多球员。
//
// 设计：rarity 分布按卡池比例
//   - 4 张 SSR
//   - 8 张 SR
//   - 10 张 R
//   - 8 张 N
var cardSeeds = []cardSeed{
	// SSR (4)
	{"Curry 23 编年史限量签名卡", "GSW", "Stephen Curry", mall.RaritySSR},
	{"James 23 终极 MVP 卡", "LAL", "LeBron James", mall.RaritySSR},
	{"Durant 35 总决赛 MVP 卡", "PHX", "Kevin Durant", mall.RaritySSR},
	{"Doncic 77 最佳新秀卡", "DAL", "Luka Doncic", mall.RaritySSR},

	// SR (8)
	{"Curry 30 全明星卡", "GSW", "Stephen Curry", mall.RaritySR},
	{"James 6 圣诞大战卡", "LAL", "LeBron James", mall.RaritySR},
	{"Tatum 0 东部决赛卡", "BOS", "Jayson Tatum", mall.RaritySR},
	{"Antetokounmpo 34 总决赛卡", "MIL", "Giannis Antetokounmpo", mall.RaritySR},
	{"Jokic 15 MVP 赛季卡", "DEN", "Nikola Jokic", mall.RaritySR},
	{"Embiid 21 得分王卡", "PHI", "Joel Embiid", mall.RaritySR},
	{"Doncic 77 最佳阵容卡", "DAL", "Luka Doncic", mall.RaritySR},
	{"Morant 12 最佳进步卡", "MEM", "Ja Morant", mall.RaritySR},

	// R (10)
	{"Curry 30 常规卡", "GSW", "Stephen Curry", mall.RarityR},
	{"James 23 常规卡", "LAL", "LeBron James", mall.RarityR},
	{"Tatum 0 常规卡", "BOS", "Jayson Tatum", mall.RarityR},
	{"Antetokounmpo 34 常规卡", "MIL", "Giannis Antetokounmpo", mall.RarityR},
	{"Jokic 15 常规卡", "DEN", "Nikola Jokic", mall.RarityR},
	{"Embiid 21 常规卡", "PHI", "Joel Embiid", mall.RarityR},
	{"Doncic 77 常规卡", "DAL", "Luka Doncic", mall.RarityR},
	{"Morant 12 常规卡", "MEM", "Ja Morant", mall.RarityR},
	{"Doncic 12 圣诞卡", "DAL", "Luka Doncic", mall.RarityR},
	{"Jokic 15 季后赛卡", "DEN", "Nikola Jokic", mall.RarityR},

	// N (8)
	{"GSW 球队卡", "GSW", "", mall.RarityN},
	{"LAL 球队卡", "LAL", "", mall.RarityN},
	{"BOS 球队卡", "BOS", "", mall.RarityN},
	{"MIL 球队卡", "MIL", "", mall.RarityN},
	{"DEN 球队卡", "DEN", "", mall.RarityN},
	{"PHI 球队卡", "PHI", "", mall.RarityN},
	{"DAL 球队卡", "DAL", "", mall.RarityN},
	{"MEM 球队卡", "MEM", "", mall.RarityN},
}

func seedCards(db *gorm.DB, now time.Time) []mall.MallCard {
	out := make([]mall.MallCard, 0, len(cardSeeds))
	for _, c := range cardSeeds {
		var existing mall.MallCard
		err := db.Where("name = ?", c.Name).First(&existing).Error
		switch {
		case err == nil:
			existing.Team = c.Team
			existing.Player = c.Player
			existing.UpdatedAt = now
			if err := db.Save(&existing).Error; err != nil {
				log.Fatalf("update card %q: %v", c.Name, err)
			}
			out = append(out, existing)
		default:
			card := mall.MallCard{
				Name: c.Name,
				Team: c.Team,
				Player: c.Player,
			}
			card.CreatedAt = now
			card.UpdatedAt = now
			if err := db.Create(&card).Error; err != nil {
				log.Fatalf("create card %q: %v", c.Name, err)
			}
			out = append(out, card)
		}
	}
	fmt.Printf("seeded cards: %d\n", len(out))
	return out
}

// =============================================================================
// 5. 盲盒（每个供应商 2 个）
// =============================================================================

type blindBoxSeed struct {
	SupplierIdx int    // 关联到 supplierSeeds 的下标
	Name        string
	Cover       string
	Price       float64
	IsFeatured  bool
}

var blindBoxSeeds = []blindBoxSeed{
	{0, "NBA 全明星盲盒", "https://cdn.example.com/kayou/nba-allstar.png", 99.00, true},
	{0, "篮球新秀盲盒", "https://cdn.example.com/kayou/rookie.png", 49.00, false},

	{1, "Panini Prizm 盲盒", "https://cdn.example.com/panini/prizm.png", 199.00, true},
	{1, "Panini Mosaic 盲盒", "https://cdn.example.com/panini/mosaic.png", 89.00, false},

	{2, "Topps Chrome 盲盒", "https://cdn.example.com/topps/chrome.png", 129.00, true},
	{2, "Topps Project70 盲盒", "https://cdn.example.com/topps/project70.png", 59.00, false},
}

func seedBlindBoxes(db *gorm.DB, now time.Time, suppliers []mall.MallSupplier) []mall.MallBlindBox {
	out := make([]mall.MallBlindBox, 0, len(blindBoxSeeds))
	for _, b := range blindBoxSeeds {
		if b.SupplierIdx >= len(suppliers) {
			log.Fatalf("blind_box %q references unknown supplier idx %d", b.Name, b.SupplierIdx)
		}
		sup := suppliers[b.SupplierIdx]
		var existing mall.MallBlindBox
		err := db.Where("supplier_id = ? AND name = ?", sup.ID, b.Name).First(&existing).Error
		switch {
		case err == nil:
			existing.Cover = b.Cover
			existing.Price = b.Price
			existing.Status = mall.StatusActive
			existing.OnSale = true
			existing.IsFeatured = b.IsFeatured
			existing.UpdatedAt = now
			if err := db.Save(&existing).Error; err != nil {
				log.Fatalf("update blind_box %q: %v", b.Name, err)
			}
			out = append(out, existing)
		default:
			box := mall.MallBlindBox{
				SupplierID: sup.ID,
				Name:       b.Name,
				Cover:      b.Cover,
				Price:      b.Price,
				Status:     mall.StatusActive,
				OnSale:     true,
				IsFeatured: b.IsFeatured,
			}
			box.CreatedAt = now
			box.UpdatedAt = now
			if err := db.Create(&box).Error; err != nil {
				log.Fatalf("create blind_box %q: %v", b.Name, err)
			}
			out = append(out, box)
		}
	}
	fmt.Printf("seeded blind_boxes: %d\n", len(out))
	return out
}

// =============================================================================
// 6. 卡池（每个盲盒 1 个）
// =============================================================================

func seedCardPools(db *gorm.DB, now time.Time, blindBoxes []mall.MallBlindBox) []mall.MallCardPool {
	out := make([]mall.MallCardPool, 0, len(blindBoxes))
	for _, bb := range blindBoxes {
		var existing mall.MallCardPool
		err := db.Where("blind_box_id = ?", bb.ID).First(&existing).Error
		switch {
		case err == nil:
			out = append(out, existing)
		default:
			pool := mall.MallCardPool{BlindBoxID: bb.ID}
			pool.CreatedAt = now
			pool.UpdatedAt = now
			if err := db.Create(&pool).Error; err != nil {
				log.Fatalf("create card_pool for blind_box %d: %v", bb.ID, err)
			}
			out = append(out, pool)
		}
	}
	fmt.Printf("seeded card_pools: %d\n", len(out))
	return out
}

// =============================================================================
// 7. 卡池条目（rarity / weight / stock）
// =============================================================================

// poolRarityPlan 每个池子按稀有度切分卡牌 + 库存。
//   - 1 张 SSR（占满 300 weight）
//   - 2 张 SR（各 600 weight，加和 1200）
//   - 3 张 R（按 3500/3 ≈ 1166.67 取整为 1166/1167/1167）
//   - 4 张 N（按 5000/4 = 1250）
//
// 加和 = 300 + 600*2 + 1166 + 1167 + 1167 + 1250*4
//      = 300 + 1200 + 3500 + 5000 = 10000 ✓
type poolRarityPlan struct {
	Rarity mall.MallCardRarity
	Count  int
	Stock  int
}

var defaultPoolPlan = []poolRarityPlan{
	{mall.RaritySSR, 1, 50},
	{mall.RaritySR, 2, 100},
	{mall.RarityR, 3, 200},
	{mall.RarityN, 4, 500},
}

// weightsByPlan 给出 defaultPoolPlan 每个 rarity 内部"均分后四舍五入到整数"的权重切片。
// 加和必须 = 该 rarity 的总权重（300/1200/3500/5000），最后一行用减法兜底。
func weightsByPlan(plan poolRarityPlan) []int {
	switch plan.Rarity {
	case mall.RaritySSR:
		return []int{300}
	case mall.RaritySR:
		return []int{600, 600}
	case mall.RarityR:
		// 1166 + 1167 + 1167 = 3500
		return []int{1166, 1167, 1167}
	case mall.RarityN:
		// 1250 × 4 = 5000
		return []int{1250, 1250, 1250, 1250}
	}
	return nil
}

func seedCardPoolItems(db *gorm.DB, now time.Time, pools []mall.MallCardPool, cards []mall.MallCard) {
	// 6 个池 × 10 卡 = 60 条 pool_items；用 (offset) 在 30 张卡里轮询，
	// 保证每个池的卡组合有差异、又不强行要求"每池都是不同 10 张"。
	const totalPerPool = 10 // 1+2+3+4 = 10

	if len(cards) < totalPerPool {
		log.Fatalf("not enough cards: have %d need %d", len(cards), totalPerPool)
	}

	for poolIdx, pool := range pools {
		// 起始 offset 让每个池的 10 张卡不同
		start := (poolIdx * 3) % len(cards)

		for planIdx, plan := range defaultPoolPlan {
			weights := weightsByPlan(plan)

			for i := 0; i < plan.Count; i++ {
				// 全局唯一索引：planIdx + i 在每个池子里按顺序取 10 张
				globalIdx := (start + planIdx*2 + i) % len(cards)
				card := cards[globalIdx]
				weight := weights[i]
				stock := plan.Stock

				var existing mall.MallCardPoolItem
				err := db.Where("pool_id = ? AND card_id = ?", pool.ID, card.ID).First(&existing).Error
				switch {
				case err == nil:
					existing.Rarity = plan.Rarity
					existing.Weight = weight
					existing.Stock = stock
					existing.UpdatedAt = now
					if err := db.Save(&existing).Error; err != nil {
						log.Fatalf("update pool_item pool=%d card=%d: %v", pool.ID, card.ID, err)
					}
				default:
					item := mall.MallCardPoolItem{
						PoolID: pool.ID,
						CardID: card.ID,
						Rarity: plan.Rarity,
						Weight: weight,
						Stock:  stock,
					}
					item.CreatedAt = now
					item.UpdatedAt = now
					if err := db.Create(&item).Error; err != nil {
						log.Fatalf("create pool_item pool=%d card=%d: %v", pool.ID, card.ID, err)
					}
				}
			}
		}
	}
	fmt.Printf("seeded card_pool_items for %d pools\n", len(pools))
}

// =============================================================================
// 8. 限时特价活动（2 个 active）
// =============================================================================

type promotionSeed struct {
	BlindBoxIdx   int
	OriginalPrice float64
	PromoPrice    float64
	// 距 now 的偏移：开始 +1h，结束 +7d，避免过期
	StartOffset time.Duration
	EndOffset   time.Duration
}

var promotionSeeds = []promotionSeed{
	{0, 99.00, 79.00, 1 * time.Hour, 7 * 24 * time.Hour},  // NBA 全明星盲盒
	{2, 199.00, 159.00, 1 * time.Hour, 7 * 24 * time.Hour}, // Panini Prizm 盲盒
}

func seedPromotions(db *gorm.DB, now time.Time, blindBoxes []mall.MallBlindBox) {
	for _, p := range promotionSeeds {
		if p.BlindBoxIdx >= len(blindBoxes) {
			log.Fatalf("promotion references unknown blind_box idx %d", p.BlindBoxIdx)
		}
		bb := blindBoxes[p.BlindBoxIdx]
		start := now.Add(p.StartOffset)
		end := now.Add(p.EndOffset)

		var existing mall.MallPromotion
		err := db.Where("blind_box_id = ? AND start_at = ?", bb.ID, start).First(&existing).Error
		switch {
		case err == nil:
			existing.OriginalPrice = p.OriginalPrice
			existing.PromoPrice = p.PromoPrice
			existing.EndAt = end
			existing.Status = mall.StatusActive
			existing.UpdatedAt = now
			if err := db.Save(&existing).Error; err != nil {
				log.Fatalf("update promotion blind_box=%d: %v", bb.ID, err)
			}
		default:
			promo := mall.MallPromotion{
				SupplierID:    bb.SupplierID,
				BlindBoxID:    bb.ID,
				OriginalPrice: p.OriginalPrice,
				PromoPrice:    p.PromoPrice,
				StartAt:       start,
				EndAt:         end,
				Status:        mall.StatusActive,
			}
			promo.CreatedAt = now
			promo.UpdatedAt = now
			if err := db.Create(&promo).Error; err != nil {
				log.Fatalf("create promotion blind_box=%d: %v", bb.ID, err)
			}
		}
	}
	fmt.Printf("seeded promotions: %d\n", len(promotionSeeds))
}

// =============================================================================
// 9. 秒杀活动（1 个 active，给前端秒杀页 / supplier 管理页做 fixture）
// =============================================================================
//
// 设计：
//   - 复用 promotionSeeds 的「活动起止偏移」：+1h 开始、+7d 结束，避免过期；
//   - 选取 idx=0 的盲盒（NBA 全明星盲盒，原价 99.00 → 秒杀 79.00），与 promotionSeeds
//     共享同一盲盒，便于联调「同一盲盒上既有活动价又有秒杀价」的两套价格展示；
//   - total_stock=100 必须 ≤ 该盲盒卡池总 stock（NBA 池：1×50 + 2×100 + 3×200 + 4×500 = 850）✓；
//   - per_user_limit=1：spec 默认值，让前端限购校验逻辑有命中场景；
//   - RedisInitialized=true：seed 不接 Redis，强行标 true 以便 supplier 后台列表 / 前端
//     秒杀页能直接展示活动（若 false 反而会触发「Redis 未 init」告警，分散调试注意力）；
//     Redis 真实库存由 seed 之后手动 SETNX 注入（README 已说明）。
//
// 幂等：与 seedPromotions 同模式 —— (blind_box_id, start_at) 复合反查，
// 重复 seed 不会产生重复活动行。

type seckillSeed struct {
	BlindBoxIdx  int
	SeckillPrice float64
	TotalStock   int
	PerUserLimit int
	StartOffset  time.Duration
	EndOffset    time.Duration
}

var seckillSeeds = []seckillSeed{
	{0, 79.00, 100, 1, 1 * time.Hour, 7 * 24 * time.Hour}, // NBA 全明星盲盒
}

func seedSeckillActivities(db *gorm.DB, now time.Time, blindBoxes []mall.MallBlindBox) {
	for _, s := range seckillSeeds {
		if s.BlindBoxIdx >= len(blindBoxes) {
			log.Fatalf("seckill references unknown blind_box idx %d", s.BlindBoxIdx)
		}
		bb := blindBoxes[s.BlindBoxIdx]
		start := now.Add(s.StartOffset)
		end := now.Add(s.EndOffset)

		var existing mall.MallSeckillActivity
		err := db.Where("blind_box_id = ? AND start_at = ?", bb.ID, start).First(&existing).Error
		switch {
		case err == nil:
			existing.SupplierID = bb.SupplierID
			existing.SeckillPrice = s.SeckillPrice
			existing.TotalStock = s.TotalStock
			existing.PerUserLimit = s.PerUserLimit
			existing.EndAt = end
			existing.Status = mall.StatusActive
			existing.RedisInitialized = true
			existing.UpdatedAt = now
			if err := db.Save(&existing).Error; err != nil {
				log.Fatalf("update seckill blind_box=%d: %v", bb.ID, err)
			}
		default:
			act := mall.MallSeckillActivity{
				SupplierID:       bb.SupplierID,
				BlindBoxID:       bb.ID,
				SeckillPrice:     s.SeckillPrice,
				TotalStock:       s.TotalStock,
				PerUserLimit:     s.PerUserLimit,
				StartAt:          start,
				EndAt:            end,
				Status:           mall.StatusActive,
				RedisInitialized: true,
			}
			act.CreatedAt = now
			act.UpdatedAt = now
			if err := db.Create(&act).Error; err != nil {
				log.Fatalf("create seckill blind_box=%d: %v", bb.ID, err)
			}
		}
	}
	fmt.Printf("seeded seckill activities: %d\n", len(seckillSeeds))
}

// =============================================================================
// 10. 历史 drawn 订单（spec 8.8：admin 结算流程联调 fixture）
// =============================================================================
//
// 设计：
//   - 给每个供应商生成若干 drawn 订单，created_at 落在上个月 [start, end) 区间，
//     用于结算预览 / Generate 时能查到 eligible orders；
//   - 价格用对应盲盒原价（活动价忽略 —— seed 不强制依赖 fixture 中是否临时下单过活动）；
//   - 通过 (supplier_id, order_no) 做唯一索引幂等：重复 seed 不产生重复数据。
//
// 注：settlement_repository.ListEligibleOrders 用 status=drawn + NOT EXISTS settlement_items，
// 所以只要这些订单还没被纳入 settlement.items 就一直可结算。

func seedHistoricalDrawnOrders(db *gorm.DB, now time.Time, blindBoxes []mall.MallBlindBox) {
	// 周期 = 上月 [1 号, 当月 1 号)，用 now 比较确定。
	periodStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -1, 0)
	periodEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	// 每个供应商 3 笔订单
	const ordersPerSupplier = 3
	total := 0

	for _, bb := range blindBoxes {
		// 只取每个供应商第一个盲盒作为下单目标，简化 fixture。
		if bb.SupplierID != blindBoxes[0].SupplierID &&
			bb.SupplierID != blindBoxes[len(blindBoxes)/2].SupplierID &&
			bb.SupplierID != blindBoxes[len(blindBoxes)-1].SupplierID {
			continue
		}
		for i := 0; i < ordersPerSupplier; i++ {
			// 时间分布在周期内（i+1）/ (ordersPerSupplier+1) 位置
			offset := time.Duration(float64(periodEnd.Sub(periodStart)) * float64(i+1) / float64(ordersPerSupplier+1))
			createdAt := periodStart.Add(offset)

			orderNo := fmt.Sprintf("seed-historical-%d-%d", bb.SupplierID, i)

			var existing mall.MallDrawOrder
			err := db.Where("order_no = ?", orderNo).First(&existing).Error
			switch {
			case err == nil:
				// 已存在 → 仅刷新 created_at / price（保持 status=drawn 让 ListEligibleOrders 还能命中）。
				existing.SupplierID = bb.SupplierID
				existing.BlindBoxID = bb.ID
				existing.Price = bb.Price
				existing.Status = mall.OrderStatusDrawn
				existing.UpdatedAt = now
				if err := db.Save(&existing).Error; err != nil {
					log.Fatalf("update historical order %q: %v", orderNo, err)
				}
			default:
				order := mall.MallDrawOrder{
					OrderNo:    orderNo,
					UserID:     1, // seed 阶段 user_id 无强约束，写常量即可
					BlindBoxID: bb.ID,
					SupplierID: bb.SupplierID,
					Price:      bb.Price,
					Status:     mall.OrderStatusDrawn,
					Source:     mall.SourceNormal,
				}
				order.CreatedAt = createdAt
				order.UpdatedAt = now
				if err := db.Create(&order).Error; err != nil {
					log.Fatalf("create historical order %q: %v", orderNo, err)
				}
			}
			total++
		}
	}

	// 累加 platform ledger counter（spec 8.5 步骤 7.6 的 fixture 化）。
	// total_revenue ≈ 总价 × 3 个盲盒 × 3 笔订单 = 9 × avgPrice；不追求精确，仅给报表一个起点。
	seedPlatformLedger(db, now)

	fmt.Printf("seeded historical drawn orders: %d (period: %s ~ %s)\n", total, periodStart.Format("2006-01-02"), periodEnd.Format("2006-01-02"))
}

// seedPlatformLedger 插入 platform ledger counter 行（admin 报表 / dashboard 用）。
//
// counter_key: 'total_revenue' 单调累加 'gmv' 视图。
// 幂等：INSERT ON DUPLICATE KEY UPDATE —— 重复 seed 时不会产生重复行。
func seedPlatformLedger(db *gorm.DB, now time.Time) {
	// 直接用 raw SQL：与 settlement_repository.IncrementLedger 共用同样 SQL。
	if err := db.Exec(
		"INSERT INTO mall_platform_ledger (counter_key, amount, created_at, updated_at) VALUES (?, ?, ?, ?) "+
			"ON DUPLICATE KEY UPDATE amount = amount + VALUES(amount), updated_at = VALUES(updated_at)",
		"total_revenue", 0.0, now, now,
	).Error; err != nil {
		// ledger seed 失败不致命 —— 业务无强依赖
		log.Printf("seed platform ledger warn: %v", err)
	}
}
