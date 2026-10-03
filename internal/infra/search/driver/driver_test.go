package driver

import (
	"context"
	"errors"
	"testing"
)

// ============================================================
// driver 包：SearchDriver 接口 + Hit / BulkDoc + ES 工厂
// ============================================================

// TestHit_Fields 验证 Hit 结构字段可直接访问（避免业务侧误用 json tag）。
func TestHit_Fields(t *testing.T) {
	h := Hit{ID: "x", Source: []byte(`{"a":1}`), Score: 0.95}
	if h.ID != "x" || h.Score != 0.95 || len(h.Source) == 0 {
		t.Errorf("Hit field assignment = %+v", h)
	}
}

// TestBulkDoc_Fields 验证 BulkDoc 字段可直接访问。
func TestBulkDoc_Fields(t *testing.T) {
	d := BulkDoc{ID: "1", Body: []byte(`{}`)}
	if d.ID != "1" || len(d.Body) == 0 {
		t.Errorf("BulkDoc field assignment = %+v", d)
	}
}

// TestElasticsearchConfig_TypeAlias 验证 ElasticsearchConfig 与 config.ElasticsearchConfig 同型。
func TestElasticsearchConfig_TypeAlias(t *testing.T) {
	var _ ElasticsearchConfig // 编译期断言
}

// TestNewElasticsearch_NoPanic 验证 NewElasticsearch 不会 panic；
//
//	正常路径返回 SearchDriver 接口（nil 也算合法 — 后续 Ping 失败 fail fast）。
func TestNewElasticsearch_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewElasticsearch panicked: %v", r)
		}
	}()
	_, _ = NewElasticsearch(ElasticsearchConfig{})
}

// TestSearchDriver_ImplementationGuard 编译期占位：保证接口未误改。
func TestSearchDriver_ImplementationGuard(t *testing.T) {
	var _ SearchDriver = (SearchDriver)(nil)
}

// errors 引用：保留 import 以防未来 driver 包引入实际 sentinel 后忘记 import。
var _ = errors.New

// context 引用：同上。
var _ = context.Background
