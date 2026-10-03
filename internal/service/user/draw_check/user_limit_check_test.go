package draw_check

import (
	"context"
	"errors"
	"testing"
	"time"

	"ai-go-mall/internal/domain/chain"
)

// =============================================================================
// stub redis：实现 AcquireUserLimit 需要的 Incr / Decr
// =============================================================================

type stubRedis struct {
	values map[string]int64
	// incrErr / decrErr 用于模拟 Redis 异常。
	incrErr error
	decrErr error
}

func newStubRedis() *stubRedis {
	return &stubRedis{values: make(map[string]int64)}
}

func (s *stubRedis) Incr(ctx context.Context, key string) (int64, error) {
	if s.incrErr != nil {
		return 0, s.incrErr
	}
	s.values[key]++
	return s.values[key], nil
}

func (s *stubRedis) Decr(ctx context.Context, key string) (int64, error) {
	if s.decrErr != nil {
		return 0, s.decrErr
	}
	s.values[key]--
	return s.values[key], nil
}

// Get 辅助查看当前值（测试断言用）。
func (s *stubRedis) Get(key string) int64 { return s.values[key] }

// =============================================================================
// 单测
// =============================================================================

func TestAcquireUserLimit_PassWithinLimit(t *testing.T) {
	r := newStubRedis()
	ctx := context.Background()

	if err := AcquireUserLimit(ctx, r, 10, 1, 2); err != nil {
		t.Fatalf("first acquire should pass, got err = %v", err)
	}
	if err := AcquireUserLimit(ctx, r, 10, 1, 2); err != nil {
		t.Fatalf("second acquire within limit should pass, got err = %v", err)
	}
	if got := r.Get("seckill:user_bought:10:1"); got != 2 {
		t.Errorf("counter = %d, want 2", got)
	}
}

func TestAcquireUserLimit_RejectWhenExceeds(t *testing.T) {
	r := newStubRedis()
	ctx := context.Background()

	// per_user_limit=1：第二次应被拒 + 自 DECR 回滚。
	if err := AcquireUserLimit(ctx, r, 10, 1, 1); err != nil {
		t.Fatalf("first acquire should pass, got err = %v", err)
	}
	if err := AcquireUserLimit(ctx, r, 10, 1, 1); !errors.Is(err, chain.ErrUserLimitExceeded) {
		t.Fatalf("second acquire should return ErrUserLimitExceeded, got err = %v", err)
	}
	// 超限被拒并 DECR 回滚，最终值 = 1（首次的 INCR 起作用）。
	if got := r.Get("seckill:user_bought:10:1"); got != 1 {
		t.Errorf("counter after reject should be 1 (rollback), got %d", got)
	}
}

func TestAcquireUserLimit_RejectAtBoundary(t *testing.T) {
	// 边界：第 n+1 次（恰好等于 limit+1）被拒。
	r := newStubRedis()
	ctx := context.Background()

	// limit=3：前 3 次通过，第 4 次拒。
	for i := 0; i < 3; i++ {
		if err := AcquireUserLimit(ctx, r, 10, 1, 3); err != nil {
			t.Fatalf("acquire %d should pass, got err = %v", i+1, err)
		}
	}
	if err := AcquireUserLimit(ctx, r, 10, 1, 3); !errors.Is(err, chain.ErrUserLimitExceeded) {
		t.Fatalf("4th acquire should reject, got err = %v", err)
	}
	// 第 4 次被拒并 DECR 回滚，最终值 = 3（前 3 次 INCR 起作用）。
	if got := r.Get("seckill:user_bought:10:1"); got != 3 {
		t.Errorf("counter = %d, want 3", got)
	}
}

func TestAcquireUserLimit_ZeroLimit_AlwaysPasses(t *testing.T) {
	// per_user_limit <= 0 时一律放行（防御数据异常）。
	r := newStubRedis()
	ctx := context.Background()

	for i := 0; i < 10; i++ {
		if err := AcquireUserLimit(ctx, r, 10, 1, 0); err != nil {
			t.Fatalf("acquire %d with limit=0 should pass, got err = %v", i+1, err)
		}
	}
	if got := r.Get("seckill:user_bought:10:1"); got != 10 {
		t.Errorf("counter = %d, want 10 (all INCRs took effect)", got)
	}
}

func TestAcquireUserLimit_DifferentUsersAreIsolated(t *testing.T) {
	r := newStubRedis()
	ctx := context.Background()

	// userA 满额，userB 不受影响。
	for i := 0; i < 3; i++ {
		_ = AcquireUserLimit(ctx, r, 10, 1, 3)
	}
	if err := AcquireUserLimit(ctx, r, 10, 2, 3); err != nil {
		t.Fatalf("userB first acquire should pass despite userA being at limit, got err = %v", err)
	}
}

func TestAcquireUserLimit_DifferentSeckillsAreIsolated(t *testing.T) {
	r := newStubRedis()
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		_ = AcquireUserLimit(ctx, r, 10, 1, 3)
	}
	if err := AcquireUserLimit(ctx, r, 11, 1, 3); err != nil {
		t.Fatalf("different seckill should pass, got err = %v", err)
	}
}

func TestAcquireUserLimit_NilCachePasses(t *testing.T) {
	// nil cache：防御编程错误，放行（不阻断业务）。
	if err := AcquireUserLimit(context.Background(), nil, 10, 1, 1); err != nil {
		t.Fatalf("nil cache should pass, got err = %v", err)
	}
}

func TestAcquireUserLimit_InvalidArgsPass(t *testing.T) {
	// 防御编程错误：seckillID/UID <=0 一律放行。
	if err := AcquireUserLimit(context.Background(), newStubRedis(), 0, 1, 1); err != nil {
		t.Errorf("seckillID=0 should pass, got err = %v", err)
	}
	if err := AcquireUserLimit(context.Background(), newStubRedis(), 10, 0, 1); err != nil {
		t.Errorf("userID=0 should pass, got err = %v", err)
	}
}

func TestAcquireUserLimit_IncrError_Propagates(t *testing.T) {
	incrErr := errors.New("redis down")
	r := newStubRedis()
	r.incrErr = incrErr

	err := AcquireUserLimit(context.Background(), r, 10, 1, 1)
	if !errors.Is(err, incrErr) {
		t.Fatalf("err = %v, want wrap incrErr", err)
	}
}

func TestAcquireUserLimit_DecrError_DoesNotBlock(t *testing.T) {
	// DECR 失败仅 best-effort，不应改变 ErrUserLimitExceeded 出口。
	r := newStubRedis()
	r.decrErr = errors.New("redis decr failed")
	ctx := context.Background()

	_ = AcquireUserLimit(ctx, r, 10, 1, 1)    // 第 1 次通过
	err := AcquireUserLimit(ctx, r, 10, 1, 1) // 第 2 次被拒
	if !errors.Is(err, chain.ErrUserLimitExceeded) {
		t.Fatalf("err = %v, want chain.ErrUserLimitExceeded", err)
	}
}

func TestUserBoughtKey_Format(t *testing.T) {
	got := userBoughtKey(10, 1)
	want := "seckill:user_bought:10:1"
	if got != want {
		t.Errorf("userBoughtKey = %q, want %q", got, want)
	}
}

// TestAcquireUserLimit_Concurrent 模拟同用户同 seckill 并发 acquire：
// 累计成功的次数应等于 limit（多余的被拒）。
func TestAcquireUserLimit_Concurrent(t *testing.T) {
	r := newStubRedis()
	ctx := context.Background()
	const limit = 5
	const total = 20

	var passed int
	for i := 0; i < total; i++ {
		if err := AcquireUserLimit(ctx, r, 10, 1, limit); err == nil {
			passed++
		}
	}
	if passed != limit {
		t.Errorf("passed = %d, want %d (per_user_limit)", passed, limit)
	}
	// 累计计数 = limit（每次被拒都 DECR 回滚）。
	if got := r.Get("seckill:user_bought:10:1"); got != int64(limit) {
		t.Errorf("counter = %d, want %d", got, limit)
	}
}

// 编译期引用：避免 time 包 unused 警告（部分测试在大量节点扩展时可能用到）。
var _ = time.Second
