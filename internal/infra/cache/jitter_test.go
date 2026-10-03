package cache

import (
	"testing"
	"time"
)

// TestJitterTTL_NonPositive 验证 base <= 0 直接返回原值。
func TestJitterTTL_NonPositive(t *testing.T) {
	cases := []time.Duration{0, -1 * time.Second, -1 * time.Hour}
	for _, base := range cases {
		if got := JitterTTL(base); got != base {
			t.Errorf("JitterTTL(%v) = %v, want %v", base, got, base)
		}
	}
}

// TestJitterTTL_Range 验证 1000 次抽样全部落在 [base*(1-jitterFraction), base*(1+jitterFraction)]。
func TestJitterTTL_Range(t *testing.T) {
	base := 100 * time.Second
	min := time.Duration(float64(base) * (1 - jitterFraction))
	max := time.Duration(float64(base) * (1 + jitterFraction))

	for i := 0; i < 1000; i++ {
		got := JitterTTL(base)
		if got < min || got > max {
			t.Fatalf("JitterTTL(%v) = %v, want in [%v, %v]", base, got, min, max)
		}
	}
}

// TestJitterTTL_NotConstant 验证抖动不为 0（即 base 自身不直接返回）。
func TestJitterTTL_NotConstant(t *testing.T) {
	base := 1 * time.Minute
	seen := make(map[time.Duration]struct{})
	for i := 0; i < 50; i++ {
		seen[JitterTTL(base)] = struct{}{}
	}
	// 50 次抽样至少出现 2 个不同值（避免退化到 no-op）。
	if len(seen) < 2 {
		t.Errorf("JitterTTL produced only %d distinct values, want >=2", len(seen))
	}
}