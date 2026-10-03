// Package cache — notfound_test.go：L1/L2 层 notFound 占位与三态 Get 语义
// （D5.1 / 任务 17.1 / 17.2）。
package cache

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai-go-mall/internal/infra/cache/driver"
)

// TestGet_ThreeState_Translation 验证 Manager / MultiLevelCache 把占位值
// 翻译成 ("", false, nil)，真实值翻译成 (v, true, nil)，miss 仍是 ErrCacheMiss。
// 每个子测试独立 fixture，避免状态串扰。
func TestGet_ThreeState_Translation(t *testing.T) {
	ctx := context.Background()

	// manager：单层，rd 即存储。
	t.Run("manager", func(t *testing.T) {
		rd := newFakeDriver("redis-fake")
		m := &Manager{driver: rd}

		if _, found, err := m.Get(ctx, "k"); !errors.Is(err, driver.ErrCacheMiss) || found {
			t.Errorf("miss = (found=%v, err=%v), want (false, ErrCacheMiss)", found, err)
		}
		_ = rd.Set(ctx, "k", `{"id":1}`, 0)
		if v, found, err := m.Get(ctx, "k"); err != nil || !found || v != `{"id":1}` {
			t.Errorf("hit = (%q, %v, %v), want (value, true, nil)", v, found, err)
		}
		_ = rd.Set(ctx, "k", NotFoundPlaceholderValue(), NotFoundPlaceholderTTL)
		if v, found, err := m.Get(ctx, "k"); err != nil || found || v != "" {
			t.Errorf("placeholder = (%q, %v, %v), want (\"\", false, nil)", v, found, err)
		}
		if _, _, err := m.Get(ctx, ""); !errors.Is(err, ErrCacheInvalid) {
			t.Errorf("empty key = %v, want ErrCacheInvalid", err)
		}
	})

	// multilevel：写走 mc.Set（双层），l2.missKeys 强制 miss。
	t.Run("multilevel", func(t *testing.T) {
		l1, l2 := newFakeDriver("l1"), newFakeDriver("l2")
		mc, err := NewMultiLevelCache(l1, l2)
		if err != nil {
			t.Fatalf("NewMultiLevelCache: %v", err)
		}

		if _, found, err := mc.Get(ctx, "k"); !errors.Is(err, driver.ErrCacheMiss) || found {
			t.Errorf("miss = (found=%v, err=%v), want (false, ErrCacheMiss)", found, err)
		}
		_ = mc.Set(ctx, "k", `{"id":1}`, 0)
		if v, found, err := mc.Get(ctx, "k"); err != nil || !found || v != `{"id":1}` {
			t.Errorf("hit = (%q, %v, %v), want (value, true, nil)", v, found, err)
		}
		_ = mc.Set(ctx, "k", NotFoundPlaceholderValue(), NotFoundPlaceholderTTL)
		if v, found, err := mc.Get(ctx, "k"); err != nil || found || v != "" {
			t.Errorf("placeholder = (%q, %v, %v), want (\"\", false, nil)", v, found, err)
		}
		// 占位写进 L1/L2 后，强制 miss 仍走 ErrCacheMiss（占位不改变 miss 语义）。
		l2.missKeys["k"] = struct{}{}
		l1.missKeys["k"] = struct{}{}
		if _, _, err := mc.Get(ctx, "k"); !errors.Is(err, driver.ErrCacheMiss) {
			t.Errorf("forced miss = %v, want ErrCacheMiss", err)
		}
		if _, _, err := mc.Get(ctx, ""); !errors.Is(err, ErrCacheInvalid) {
			t.Errorf("empty key = %v, want ErrCacheInvalid", err)
		}
	})
}

// TestNotFoundPlaceholder_TTLConstant 验证占位 TTL 常量（D5.1：30s）。
func TestNotFoundPlaceholder_TTLConstant(t *testing.T) {
	if NotFoundPlaceholderTTL != 30*time.Second {
		t.Errorf("NotFoundPlaceholderTTL = %v, want 30s", NotFoundPlaceholderTTL)
	}
	if NotFoundPlaceholderValue() != `{"_notFound":true}` {
		t.Errorf("placeholder value = %q", NotFoundPlaceholderValue())
	}
	if !IsNotFoundValue(NotFoundPlaceholderValue()) {
		t.Error("IsNotFoundValue(placeholder) = false")
	}
}
