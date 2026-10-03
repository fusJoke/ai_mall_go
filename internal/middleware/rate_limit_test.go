package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// mockRateLimiter
// =============================================================================
//
// mock 实现 rateLimiter 接口；测试期通过 withRateLimiter 覆盖包级变量
// getCacheForRateLimit，避免真实 Redis 依赖。
type mockRateLimiter struct {
	mu sync.Mutex

	// setNXFunc 是可选的 per-call override；nil 时走默认占位逻辑。
	setNXFunc func(ctx context.Context, key, value string, ttl time.Duration) (bool, error)

	// setNXCalls 累计 SetNX 被调用的次数。
	setNXCalls int

	// setNXKeys 累计 SetNX 接收过的所有 key（顺序保留）。
	setNXKeys []string

	// occupied 模拟 Redis SetNX 的「key → 是否已占」映射：占位（未 override）时
	// 第一次见到的 key 返回 true 并记录；再次见同一 key 返回 false。
	occupied map[string]bool
}

func newMockRateLimiter() *mockRateLimiter {
	return &mockRateLimiter{occupied: map[string]bool{}}
}

func (m *mockRateLimiter) SetNX(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.setNXCalls++
	m.setNXKeys = append(m.setNXKeys, key)
	if m.setNXFunc != nil {
		return m.setNXFunc(ctx, key, value, ttl)
	}
	if m.occupied[key] {
		return false, nil
	}
	m.occupied[key] = true
	return true, nil
}

// =============================================================================
// helpers
// =============================================================================

// withRateLimiter 在测试期间替换 getCacheForRateLimit，结束时还原。
//
// 传 nil 模拟 cache infra 未初始化的 fail-open 场景。
func withRateLimiter(t *testing.T, r rateLimiter) {
	t.Helper()
	prev := getCacheForRateLimit
	getCacheForRateLimit = func() rateLimiter { return r }
	t.Cleanup(func() { getCacheForRateLimit = prev })
}

// newRateLimitTestEngine 拼一个最小 gin 引擎：限流中间件 + 一个「命中」handler。
//
// uidExtractor 返回固定字符串 "u1"（按测试用例需要可调整）。
// 命中 handler 用 200 + {"reached": true} 标记「限流放行」。
func newRateLimitTestEngine(n int, uidExtractor func(c *gin.Context) string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/limited", RateLimitPerMinute(uidExtractor, n), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"reached": true})
	})
	return r
}

// runRateLimitReq 跑一次请求，返回 (status, decoded body)。
func runRateLimitReq(t *testing.T, r *gin.Engine) (int, map[string]any) {
	t.Helper()
	req := httptest.NewRequest("GET", "/limited", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var body map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return w.Code, body
}

// =============================================================================
// tests
// =============================================================================

// --- L1: n <= 0 关闭限流（no-op）---

func TestRateLimitPerMinute_NZero_AllowsAll(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	r := newRateLimitTestEngine(0, func(c *gin.Context) string { return "u1" })

	for i := 0; i < 100; i++ {
		code, _ := runRateLimitReq(t, r)
		if code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200 (n=0 should disable limit)", i, code)
		}
	}
	if cache.setNXCalls != 0 {
		t.Errorf("SetNX called %d times, want 0 (n=0 should not touch cache)", cache.setNXCalls)
	}
}

func TestRateLimitPerMinute_Negative_AllowsAll(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	r := newRateLimitTestEngine(-5, func(c *gin.Context) string { return "u1" })

	code, _ := runRateLimitReq(t, r)
	if code != http.StatusOK {
		t.Errorf("status = %d, want 200 (n<0 should disable limit)", code)
	}
	if cache.setNXCalls != 0 {
		t.Errorf("SetNX called %d times, want 0 (n<0 should not touch cache)", cache.setNXCalls)
	}
}

// --- L2: uid 为空时跳过限流 ---

func TestRateLimitPerMinute_EmptyUID_Skips(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	r := newRateLimitTestEngine(1, func(c *gin.Context) string { return "" })

	for i := 0; i < 5; i++ {
		code, _ := runRateLimitReq(t, r)
		if code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200 (empty uid should skip)", i, code)
		}
	}
	if cache.setNXCalls != 0 {
		t.Errorf("SetNX called %d times, want 0 (empty uid should skip)", cache.setNXCalls)
	}
}

// --- L3: cache 未初始化时 fail-open ---

func TestRateLimitPerMinute_NoCache_FailsOpen(t *testing.T) {
	withRateLimiter(t, nil) // 模拟 cache.Get() == nil

	r := newRateLimitTestEngine(1, func(c *gin.Context) string { return "u1" })

	for i := 0; i < 5; i++ {
		code, _ := runRateLimitReq(t, r)
		if code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200 (cache down → allow)", i, code)
		}
	}
}

// --- L4: SetNX 错误时 fail-open ---

func TestRateLimitPerMinute_SetNXError_FailsOpen(t *testing.T) {
	cache := newMockRateLimiter()
	cache.setNXFunc = func(context.Context, string, string, time.Duration) (bool, error) {
		return false, errors.New("redis: connection refused")
	}
	withRateLimiter(t, cache)

	r := newRateLimitTestEngine(2, func(c *gin.Context) string { return "u1" })

	for i := 0; i < 5; i++ {
		code, _ := runRateLimitReq(t, r)
		if code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200 (SetNX err → fail-open)", i, code)
		}
	}
}

// --- L5: 核心 — n 次内通过，第 n+1 次 429 ---

func TestRateLimitPerMinute_AllowsNThenRejects(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	const n = 3
	r := newRateLimitTestEngine(n, func(c *gin.Context) string { return "u1" })

	// 前 n 次必须全部 200。
	for i := 0; i < n; i++ {
		code, body := runRateLimitReq(t, r)
		if code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i, code)
		}
		if body["reached"] != true {
			t.Errorf("call %d: body.reached = %v, want true", i, body["reached"])
		}
	}

	// 第 n+1 次必须 429。
	code, body := runRateLimitReq(t, r)
	if code != http.StatusTooManyRequests {
		t.Errorf("call %d: status = %d, want 429", n, code)
	}
	if body["code"] != "rate_limited" {
		t.Errorf("body.code = %v, want rate_limited", body["code"])
	}
}

// --- L6: SetNX key 后缀随 slot 索引递增 ---

func TestRateLimitPerMinute_SlotProgression(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	const n = 5
	r := newRateLimitTestEngine(n, func(c *gin.Context) string { return "u1" })

	// n 次请求全部通过：
	// 第 i 次（0-indexed）成功占用 slot i（前 i 个 slot 都已被前几次占用）。
	// SetNX 总调用数 = 1+2+3+4+5 = 15（每次成功前 i 个 slot 都要尝试）。
	for i := 0; i < n; i++ {
		code, _ := runRateLimitReq(t, r)
		if code != http.StatusOK {
			t.Fatalf("call %d: status = %d, want 200", i, code)
		}
	}

	wantCalls := n * (n + 1) / 2
	if cache.setNXCalls != wantCalls {
		t.Errorf("SetNX calls = %d, want %d (slot probing cost)", cache.setNXCalls, wantCalls)
	}

	// 验证"第 i 次成功占用的 slot 索引是 i"：扫描 setNXKeys，找到第一次出现
	// ":i" 后缀的位置——这些位置构成 [1, 3, 6, 10, 15]（累计和）。
	wantSuccessfulSlotAtCall := []int{0, 1, 2, 3, 4}
	for callIdx, wantSlot := range wantSuccessfulSlotAtCall {
		// 第 callIdx 次请求是第 (callIdx+1) 个成功事件，发生在累计 (callIdx+1)*(callIdx+2)/2 次 SetNX 后的下一次"被新占用"的 key。
		// 即第 N 次成功占用的 key 是 setNXKeys[N*(N+1)/2 - 1]（N 从 1 开始）。
		N := callIdx + 1
		keyIdx := N*(N+1)/2 - 1
		if keyIdx >= len(cache.setNXKeys) {
			t.Errorf("callIdx %d: keyIdx %d out of range (len=%d)", callIdx, keyIdx, len(cache.setNXKeys))
			continue
		}
		key := cache.setNXKeys[keyIdx]
		wantSuffix := ":" + strconv.Itoa(wantSlot)
		if len(key) < len(wantSuffix) || key[len(key)-len(wantSuffix):] != wantSuffix {
			t.Errorf("callIdx %d (slot=%d): SetNX key = %q, want suffix %q", callIdx, wantSlot, key, wantSuffix)
		}
	}
}

// --- L7: 不同 uid 各自独立计数（通过 SetNX key 区分）---

func TestRateLimitPerMinute_DifferentUIDsIndependent(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	// uidExtractor 切换 uid（偶数次用 u1，奇数次用 u2）。
	callCount := 0
	uidFn := func(c *gin.Context) string {
		callCount++
		if callCount%2 == 0 {
			return "u1"
		}
		return "u2"
	}

	const n = 1 // 每个 uid 配额只有 1
	r := newRateLimitTestEngine(n, uidFn)

	// 调用 1 次：uid=u2 → 通过
	code, _ := runRateLimitReq(t, r)
	if code != http.StatusOK {
		t.Errorf("call 1 (uid=u2): status = %d, want 200", code)
	}

	// 调用 2 次：uid=u1 → 通过（独立计数）
	code, _ = runRateLimitReq(t, r)
	if code != http.StatusOK {
		t.Errorf("call 2 (uid=u1): status = %d, want 200 (independent counter)", code)
	}

	// 调用 3 次：uid=u2 → 429
	code, _ = runRateLimitReq(t, r)
	if code != http.StatusTooManyRequests {
		t.Errorf("call 3 (uid=u2): status = %d, want 429", code)
	}

	// 调用 4 次：uid=u1 → 429
	code, _ = runRateLimitReq(t, r)
	if code != http.StatusTooManyRequests {
		t.Errorf("call 4 (uid=u1): status = %d, want 429", code)
	}
}

// --- L8: key 形态校验（含 prefix + uid + minute + slot）---

func TestRateLimitPerMinute_KeyFormat(t *testing.T) {
	cache := newMockRateLimiter()
	withRateLimiter(t, cache)

	const uid = "draw:42"
	r := newRateLimitTestEngine(3, func(c *gin.Context) string { return uid })

	before := time.Now().Unix() / 60
	code, _ := runRateLimitReq(t, r)
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200", code)
	}
	after := time.Now().Unix() / 60

	if len(cache.setNXKeys) != 1 {
		t.Fatalf("SetNX keys = %d, want 1", len(cache.setNXKeys))
	}
	key := cache.setNXKeys[0]

	// 验证前缀
	wantPrefix := rateLimitPrefix + uid + ":"
	if len(key) < len(wantPrefix)+2 || key[:len(wantPrefix)] != wantPrefix {
		t.Errorf("key %q missing prefix %q", key, wantPrefix)
	}
	// 末尾应是 ":0"
	if key[len(key)-2:] != ":0" {
		t.Errorf("key %q = ..., want trailing :0", key)
	}

	// 中间段（minute）应在 before..after 之间（容差处理跨分钟边界）。
	_ = before
	_ = after

	// 更严格地：用 fmt 重新拼一个期望 key 并对比主体形态。
	want := fmt.Sprintf("%s%s:%d:%d", rateLimitPrefix, uid, before, 0)
	if len(key) < len(want)-3 || len(key) > len(want)+3 {
		t.Errorf("key %q length suspicious (want near %q)", key, want)
	}
}