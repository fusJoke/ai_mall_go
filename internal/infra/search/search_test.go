package search

import (
	"context"
	"errors"
	"testing"
)

// ============================================================
// search 包：参数校验 + 包级状态（无 ES 依赖）
// ============================================================

// TestErrSearchInvalid_Stable 验证包 sentinel 非空。
func TestErrSearchInvalid_Stable(t *testing.T) {
	if ErrSearchInvalid == nil {
		t.Fatal("ErrSearchInvalid must be non-nil")
	}
	if ErrSearchInvalid.Error() == "" {
		t.Errorf("ErrSearchInvalid.Error() empty")
	}
}

// TestManager_NilBeforeInit 验证未 Init 时 Get() 返回 nil。
func TestManager_NilBeforeInit(t *testing.T) {
	Reset() // 兜底
	if g := Get(); g != nil {
		t.Errorf("Get() before Init = %v, want nil", g)
	}
}

// TestManager_InvalidArgsRejected 验证 Manager.Index / BulkIndex / Delete / Search
// 收到非法入参直接返回 ErrSearchInvalid，不调底层 driver（driver=nil 时不应 NPE）。
func TestManager_InvalidArgsRejected(t *testing.T) {
	m := &Manager{} // driver=nil
	ctx := context.Background()
	body := []byte(`{}`)

	// Index: 空 index / 空 id 都拦
	if err := m.Index(ctx, "", "1", body); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("Index empty index = %v, want ErrSearchInvalid", err)
	}
	if err := m.Index(ctx, "idx", "", body); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("Index empty id = %v, want ErrSearchInvalid", err)
	}

	// BulkIndex: 空 index / 空 docs 都拦
	if err := m.BulkIndex(ctx, "", nil); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("BulkIndex empty index = %v, want ErrSearchInvalid", err)
	}
	if err := m.BulkIndex(ctx, "idx", nil); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("BulkIndex empty docs = %v, want ErrSearchInvalid", err)
	}
	if err := m.BulkIndex(ctx, "idx", []BulkDoc{}); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("BulkIndex empty slice = %v, want ErrSearchInvalid", err)
	}

	// Delete: 空 index / 空 id 都拦
	if err := m.Delete(ctx, "", "1"); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("Delete empty index = %v, want ErrSearchInvalid", err)
	}
	if err := m.Delete(ctx, "idx", ""); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("Delete empty id = %v, want ErrSearchInvalid", err)
	}

	// Search: 空 index 拦；空 body 不拦（业务允许 "*" / match_all）
	if _, _, err := m.Search(ctx, "", body); !errors.Is(err, ErrSearchInvalid) {
		t.Errorf("Search empty index = %v, want ErrSearchInvalid", err)
	}
}

// TestReset_ClearsMgr 验证 Reset() 后 Get() 返回 nil。
func TestReset_ClearsMgr(t *testing.T) {
	mgr = &Manager{}
	Reset()
	if g := Get(); g != nil {
		t.Errorf("Get() after Reset = %v, want nil", g)
	}
}

// TestTypeAliases 验证 Hit / BulkDoc 是 type alias（保证驱动切换不破坏业务侧类型断言）。
func TestTypeAliases(t *testing.T) {
	var _ Hit     // 编译期必须解析为 driver.Hit
	var _ BulkDoc // 编译期必须解析为 driver.BulkDoc
}

// TestSearchClient_ImplementationGuard 编译期确保 Manager 满足 SearchClient 接口。
func TestSearchClient_ImplementationGuard(t *testing.T) {
	var _ SearchClient = (*Manager)(nil)
}
