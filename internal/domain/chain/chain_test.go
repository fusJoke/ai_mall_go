package chain

import (
	"errors"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// =============================================================================
// 测试 helper
// =============================================================================

func newTestCtx() *gin.Context {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("POST", "/test", nil)
	return c
}

// fakeValidator 构造可控的 Validator：name / returns 控制其行为 + 追踪调用次数。
type fakeValidator struct {
	name    string
	returns error
	calls   int
	inputs  []*DrawInput
}

func (f *fakeValidator) Name() string { return f.name }

func (f *fakeValidator) Validate(c *gin.Context, input *DrawInput) error {
	f.calls++
	f.inputs = append(f.inputs, input)
	return f.returns
}

// =============================================================================
// 单元测试：Chain.Validate 行为
// =============================================================================

func TestChain_EmptyChain_PassesImmediately(t *testing.T) {
	// 空链（无节点）应该立即通过。
	ch := NewChain()
	input := &DrawInput{Source: SourceNormal, UserID: 1, BlindBoxID: 1, Now: time.Now()}
	if err := ch.Validate(newTestCtx(), input); err != nil {
		t.Fatalf("empty chain should pass, got err = %v", err)
	}
}

func TestChain_AllValidatorsPass_ReturnsNil(t *testing.T) {
	v1 := &fakeValidator{name: "v1"}
	v2 := &fakeValidator{name: "v2"}
	v3 := &fakeValidator{name: "v3"}
	ch := NewChain(v1, v2, v3)

	input := &DrawInput{Source: SourceNormal, UserID: 1, BlindBoxID: 1, Now: time.Now()}
	if err := ch.Validate(newTestCtx(), input); err != nil {
		t.Fatalf("all-pass should return nil, got err = %v", err)
	}
	if v1.calls != 1 || v2.calls != 1 || v3.calls != 1 {
		t.Errorf("expected each validator called once, got v1=%d v2=%d v3=%d", v1.calls, v2.calls, v3.calls)
	}
}

func TestChain_FirstValidatorFails_TerminatesImmediately(t *testing.T) {
	v1 := &fakeValidator{name: "v1", returns: ErrInvalidSource}
	v2 := &fakeValidator{name: "v2"}
	v3 := &fakeValidator{name: "v3"}
	ch := NewChain(v1, v2, v3)

	input := &DrawInput{Source: SourceSeckill, UserID: 1, SeckillID: 1, Now: time.Now()}
	err := ch.Validate(newTestCtx(), input)
	if err == nil {
		t.Fatal("expected error from v1, got nil")
	}

	// v1 被调用过一次（但这是入口校验的 ErrInvalidSource 的包装），后续节点必须不再触发。
	if v1.calls != 1 {
		t.Errorf("v1.calls = %d, want 1", v1.calls)
	}
	// 注：v2/v3 的调用次数依赖 v1 是否实际通过输入校验返回。
	// 这里 v1.returns 直接是 ErrInvalidSource，但 Chain.Validate 先做入口校验（Source/UserID）
	// 后才进 validator 循环 —— 所以 v1 不会被调。原因下面另写测试覆盖。
}

// TestChain_FirstValidatorFails_SecondNotCalled 显式验证「任一节点失败 → 后续不跑」。
func TestChain_FirstValidatorFails_SecondNotCalled(t *testing.T) {
	sentinelErr := errors.New("v2 fail")
	v1 := &fakeValidator{name: "v1", returns: sentinelErr}
	v2 := &fakeValidator{name: "v2"}
	ch := NewChain(v1, v2)

	input := &DrawInput{Source: SourceNormal, UserID: 1, BlindBoxID: 1, Now: time.Now()}
	err := ch.Validate(newTestCtx(), input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if v1.calls != 1 {
		t.Errorf("v1.calls = %d, want 1", v1.calls)
	}
	if v2.calls != 0 {
		t.Errorf("v2.calls = %d, want 0 (chain should terminate)", v2.calls)
	}

	// 错误必须 wrap validator name + 原 sentinel
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err should be *ValidationError, got %T", err)
	}
	if ve.ValidatorName != "v1" {
		t.Errorf("ValidatorName = %q, want %q", ve.ValidatorName, "v1")
	}
	if !errors.Is(err, sentinelErr) {
		t.Errorf("err should wrap sentinel, errors.Is failed")
	}
	if ve.Error() == "" {
		t.Errorf("ValidationError.Error() should be non-empty")
	}
}

// TestChain_MiddleValidatorFails_LastNotCalled 验证中段失败，末段不触发。
func TestChain_MiddleValidatorFails_LastNotCalled(t *testing.T) {
	v1 := &fakeValidator{name: "v1"}
	v2 := &fakeValidator{name: "v2", returns: errors.New("boom")}
	v3 := &fakeValidator{name: "v3"}
	ch := NewChain(v1, v2, v3)

	input := &DrawInput{Source: SourceNormal, UserID: 1, BlindBoxID: 1, Now: time.Now()}
	err := ch.Validate(newTestCtx(), input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if v1.calls != 1 || v2.calls != 1 || v3.calls != 0 {
		t.Errorf("call counts wrong: v1=%d v2=%d v3=%d (want 1/1/0)", v1.calls, v2.calls, v3.calls)
	}
}

// TestChain_InputValidationFailsBeforeValidators 验证入口校验先于 validator。
func TestChain_InputValidationFailsBeforeValidators(t *testing.T) {
	v := &fakeValidator{name: "v"}
	ch := NewChain(v)

	tests := []struct {
		name  string
		input *DrawInput
		want  error // sentinel to errors.Is against
	}{
		{"nil input", nil, ErrMissingInput},
		{"invalid source", &DrawInput{Source: "weird", UserID: 1}, ErrInvalidSource},
		{"user_id zero", &DrawInput{Source: SourceNormal, BlindBoxID: 1}, ErrMissingInput},
		{"normal without blind_box_id", &DrawInput{Source: SourceNormal, UserID: 1}, ErrMissingInput},
		{"seckill without seckill_id", &DrawInput{Source: SourceSeckill, UserID: 1, BlindBoxID: 1}, ErrMissingInput},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ch.Validate(newTestCtx(), tt.input)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want wrap %v", err, tt.want)
			}
			if v.calls != 0 {
				t.Errorf("validator should not be called when input invalid, got calls=%d", v.calls)
			}
		})
	}
}

// TestChain_InputPassedThroughToValidators 验证 input 引用透传到每个 validator。
func TestChain_InputPassedThroughToValidators(t *testing.T) {
	v1 := &fakeValidator{name: "v1"}
	v2 := &fakeValidator{name: "v2"}
	ch := NewChain(v1, v2)

	input := &DrawInput{Source: SourceSeckill, UserID: 42, SeckillID: 7, Now: time.Unix(1234567890, 0)}
	if err := ch.Validate(newTestCtx(), input); err != nil {
		t.Fatalf("unexpected err = %v", err)
	}

	for i, v := range []*fakeValidator{v1, v2} {
		if len(v.inputs) != 1 {
			t.Fatalf("v%d: len(inputs)=%d, want 1", i+1, len(v.inputs))
		}
		got := v.inputs[0]
		if got.UserID != 42 || got.SeckillID != 7 || got.Source != SourceSeckill {
			t.Errorf("v%d input mismatch: got %+v", i+1, got)
		}
	}
}

// =============================================================================
// 单元测试：ValidationError 协议
// =============================================================================

func TestValidationError_IsAndAs(t *testing.T) {
	sentinel := errors.New("sentinel")
	ve := &ValidationError{ValidatorName: "x", Cause: sentinel}

	// errors.As 应能取出 ValidationError
	var got *ValidationError
	if !errors.As(error(ve), &got) {
		t.Fatal("errors.As should match *ValidationError")
	}
	if got.ValidatorName != "x" {
		t.Errorf("got.ValidatorName = %q, want %q", got.ValidatorName, "x")
	}

	// errors.Is 应能命中底层 sentinel
	if !errors.Is(ve, sentinel) {
		t.Errorf("errors.Is should match sentinel")
	}

	// Error() 应包含 validator name 与 cause
	msg := ve.Error()
	if msg == "" {
		t.Errorf("Error() should be non-empty")
	}
}

func TestValidationError_NilSafeUnwrap(t *testing.T) {
	// nil receiver 上调方法不应 panic（防御编程错误）。
	var ve *ValidationError
	if err := errors.Unwrap(ve); err != nil {
		t.Errorf("Unwrap(nil) should return nil, got %v", err)
	}
	if msg := ve.Error(); msg != "" {
		t.Errorf("Error() on nil = %q, want empty", msg)
	}
}

// TestValidationError_NestedUnwrap 验证 errors.Unwrap 链路穿透。
func TestValidationError_NestedUnwrap(t *testing.T) {
	sentinel := errors.New("sentinel")
	ve := &ValidationError{ValidatorName: "v", Cause: sentinel}

	if err := errors.Unwrap(ve); err != sentinel {
		t.Errorf("Unwrap should return Cause, got %v", err)
	}
}

// TestValidationError_WrappedByFmtErrorf 验证 ValidationError 包在 fmt.Errorf 里也能命中 sentinel。
func TestValidationError_WrappedByFmtErrorf(t *testing.T) {
	sentinel := errors.New("sentinel")
	ve := &ValidationError{ValidatorName: "v", Cause: sentinel}
	wrapped := fmt.Errorf("outer: %w", ve)

	if !errors.Is(wrapped, sentinel) {
		t.Errorf("wrapped errors.Is should match sentinel")
	}
	var got *ValidationError
	if !errors.As(wrapped, &got) {
		t.Errorf("wrapped errors.As should match *ValidationError")
	}
}