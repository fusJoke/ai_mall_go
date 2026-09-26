package driver

import (
	"strings"
	"testing"
)

func TestHashToken_StableAnd64Chars(t *testing.T) {
	// SHA256 hex 固定 64 字符；同一输入必须产出同一输出。
	got1 := HashToken("abc")
	got2 := HashToken("abc")
	if got1 != got2 {
		t.Errorf("HashToken not stable: %q vs %q", got1, got2)
	}
	if len(got1) != 64 {
		t.Errorf("HashToken length = %d, want 64", len(got1))
	}
	// 输出只含 hex 字符（小写）。
	for _, r := range got1 {
		if !strings.ContainsRune("0123456789abcdef", r) {
			t.Errorf("HashToken contains non-hex char %q in %q", r, got1)
		}
	}
}

func TestHashToken_DistinctInputs(t *testing.T) {
	// 不同明文产出不同哈希。
	if HashToken("abc") == HashToken("abd") {
		t.Errorf("HashToken(abc) == HashToken(abd); expected distinct")
	}
	if HashToken("") == HashToken("abc") {
		t.Errorf("HashToken(\"\") == HashToken(\"abc\"); expected distinct")
	}
}

// TestHashToken_KnownVector 用经典测试向量验证算法正确性：
// SHA256("abc") = ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad。
func TestHashToken_KnownVector(t *testing.T) {
	got := HashToken("abc")
	want := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Errorf("HashToken(\"abc\") = %q, want %q", got, want)
	}
}

func TestNewDatabase_StoresBaseDB(t *testing.T) {
	// NewDatabase 仅缓存 base，nil 也允许（Init 测试场景需要）。
	// 这里只验证返回非 nil + 字段透传。
	d := NewDatabase(nil)
	if d == nil {
		t.Fatal("NewDatabase(nil) = nil, want non-nil wrapper")
	}
	if d.base != nil {
		t.Errorf("base should be nil when constructed with nil, got %v", d.base)
	}
}
