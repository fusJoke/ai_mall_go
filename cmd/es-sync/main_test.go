package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"ai-go-mall/internal/model/mall"
	userSvc "ai-go-mall/internal/service/user"
)

func TestBuildRaritySummary(t *testing.T) {
	items := []mall.MallCardPoolItem{
		{Rarity: mall.RaritySSR, Weight: 300},
		{Rarity: mall.RaritySR, Weight: 1200},
		{Rarity: mall.RarityR, Weight: 3500},
		{Rarity: mall.RarityN, Weight: 5000},
	}
	got := buildRaritySummary(items)
	want := "SSR 3% / SR 12% / R 35% / N 50%"
	if got != want {
		t.Errorf("buildRaritySummary = %q, want %q", got, want)
	}
}

func TestBuildRaritySummary_SkipsZeroWeight(t *testing.T) {
	items := []mall.MallCardPoolItem{
		{Rarity: mall.RaritySSR, Weight: 0},
		{Rarity: mall.RarityN, Weight: 10000},
	}
	got := buildRaritySummary(items)
	if got != "N 100%" {
		t.Errorf("buildRaritySummary = %q, want %q", got, "N 100%")
	}
}

func TestBuildRaritySummary_Empty(t *testing.T) {
	if got := buildRaritySummary(nil); got != "" {
		t.Errorf("buildRaritySummary(nil) = %q, want empty", got)
	}
}

func TestProjectBlindBox(t *testing.T) {
	doc := projectBlindBox(blindBoxInput{
		ID:           1,
		SupplierID:   10,
		SupplierName: "Panini 旗舰店",
		Name:         "2024 NBA 球星卡盲盒",
		Cover:        "https://cdn.example.com/cover.jpg",
		Price:        99,
		PromoPrice:   promoPriceOf(79),
		Items: []mall.MallCardPoolItem{
			{Rarity: mall.RaritySSR, Weight: 300},
			{Rarity: mall.RarityN, Weight: 9700},
		},
		CreatedAt: time.Unix(1791024000, 0).UTC(),
	})

	if doc.ID != 1 || doc.SupplierID != 10 || doc.SupplierName != "Panini 旗舰店" {
		t.Errorf("doc identity = %+v", doc)
	}
	if doc.Price != 99 {
		t.Errorf("price = %v", doc.Price)
	}
	if doc.PromoPrice == nil || *doc.PromoPrice != 79 {
		t.Errorf("promo_price = %v, want 79", doc.PromoPrice)
	}
	if !strings.Contains(doc.RaritySummary, "SSR 3%") || !strings.Contains(doc.RaritySummary, "N 97%") {
		t.Errorf("rarity_summary = %q", doc.RaritySummary)
	}
	if doc.HotScore != 0 {
		t.Errorf("hot_score = %d, want 0 (MVP 无来源，默认 0)", doc.HotScore)
	}

	// JSON 字段名必须是 snake_case（与 service/user/home.go 的反序列化 DTO 对齐）。
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, key := range []string{"id", "supplier_id", "supplier_name", "name", "cover", "price", "rarity_summary", "hot_score", "created_at"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing json key %q in %s", key, b)
		}
	}
	if _, has := raw["promo_price"]; !has {
		t.Errorf("promo_price key missing when set: %s", b)
	}
}

func TestProjectBlindBox_NoPromo_OmitsField(t *testing.T) {
	doc := projectBlindBox(blindBoxInput{
		ID: 2, SupplierID: 10, Name: "X", Price: 59, CreatedAt: time.Unix(1791024000, 0).UTC(),
	})
	b, _ := json.Marshal(doc)
	if strings.Contains(string(b), "promo_price") {
		t.Errorf("promo_price should be omitted when no active promotion: %s", b)
	}
}

func TestProjectSupplier(t *testing.T) {
	doc := projectSupplier(supplierInput{
		ID: 10, Name: "Panini 旗舰店", Logo: "https://cdn/logo.png", BlindBoxCount: 3,
	})
	if doc.ID != 10 || doc.BlindBoxCount != 3 {
		t.Errorf("doc = %+v", doc)
	}
	if doc.FeaturedRank != 0 {
		t.Errorf("featured_rank = %d, want 0 (MVP 无来源，默认 0)", doc.FeaturedRank)
	}
	b, _ := json.Marshal(doc)
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	for _, key := range []string{"id", "name", "logo", "blind_box_count", "featured_rank"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("missing json key %q in %s", key, b)
		}
	}
}

// TestProjectBlindBox_RoundTripIntoFeedDTO 回归守卫（final review Critical #2）：
// es-sync 写入 ES 的文档 JSON 必须能被 service/user.home 的 BlindBoxFeedItem
// 原样反序列化 —— created_at 曾因写成 unix 数字导致 Feed 500，本测试保证
// 两端类型永远对齐。
func TestProjectBlindBox_RoundTripIntoFeedDTO(t *testing.T) {
	doc := projectBlindBox(blindBoxInput{
		ID:           1,
		SupplierID:   10,
		SupplierName: "Panini 旗舰店",
		Name:         "NBA 盲盒",
		Price:        99,
		PromoPrice:   promoPriceOf(79),
		CreatedAt:    time.Unix(1791024000, 0).UTC(),
	})
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var feedItem userSvc.BlindBoxFeedItem
	if err := json.Unmarshal(b, &feedItem); err != nil {
		t.Fatalf("unmarshal into home.BlindBoxFeedItem: %v (created_at 类型失配?)", err)
	}
	if feedItem.ID != 1 || feedItem.SupplierName != "Panini 旗舰店" || feedItem.CreatedAt.IsZero() {
		t.Errorf("feedItem = %+v", feedItem)
	}
}

// TestProjectSupplier_RoundTripIntoFeedDTO 供应商文档同理。
func TestProjectSupplier_RoundTripIntoFeedDTO(t *testing.T) {
	b, err := json.Marshal(projectSupplier(supplierInput{ID: 10, Name: "X", BlindBoxCount: 3}))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var feedItem userSvc.SupplierFeedItem
	if err := json.Unmarshal(b, &feedItem); err != nil {
		t.Fatalf("unmarshal into home.SupplierFeedItem: %v", err)
	}
	if feedItem.ID != 10 || feedItem.BlindBoxCount != 3 {
		t.Errorf("feedItem = %+v", feedItem)
	}
}

// TestFilterBoxesOfActiveSuppliers 回归（final review Important #5）：
// 被禁供应商的盲盒不得进入 ES feed（spec "Admin disables a supplier"）。
func TestFilterBoxesOfActiveSuppliers(t *testing.T) {
	boxes := []mall.MallBlindBox{
		{ID: 1, SupplierID: 10},
		{ID: 2, SupplierID: 20},
		{ID: 3, SupplierID: 10},
	}
	active := map[int64]struct{}{10: {}}

	got := filterBoxesOfActiveSuppliers(boxes, active)
	if len(got) != 2 || got[0].ID != 1 || got[1].ID != 3 {
		t.Errorf("filtered = %+v, want boxes of supplier 10 only", got)
	}
}
