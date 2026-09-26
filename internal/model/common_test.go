package model

import (
	"reflect"
	"testing"

	"gorm.io/gorm/schema"
)

func TestToken_Registered(t *testing.T) {
	// init() 已经把 Token 注册到全局表（包加载期执行，不依赖测试环境）。
	// 注意：不要在这里调 Reset() —— Reset 会清空注册表且不会重新触发 init()，
	// 会让其他测试（如 model_test.go 里的 TestRegister_AndAll）也跟着失败。
	all := All()
	got := map[reflect.Type]bool{}
	for _, m := range all {
		got[typeOf(m)] = true
	}
	if !got[reflect.TypeOf(Token{})] {
		t.Fatalf("expected Token to be registered, got %v", all)
	}
}

func TestToken_TableName_NoPrefix(t *testing.T) {
	// 无前缀时表名就是 "tokens"。
	namer := schema.NamingStrategy{}
	got := (Token{}).TableName(namer)
	want := "tokens"
	if got != want {
		t.Errorf("TableName() = %q, want %q", got, want)
	}
}

func TestToken_TableName_WithPrefix(t *testing.T) {
	// 带前缀时必须正确拼上，确保 database.prefix 生效。
	namer := schema.NamingStrategy{TablePrefix: "mall_"}
	got := (Token{}).TableName(namer)
	want := "mall_tokens"
	if got != want {
		t.Errorf("TableName() with prefix = %q, want %q", got, want)
	}
}

// 确保 Token 实现了 schema.Tabler（间接通过 TablerWithNamer）。
// 这条是编译期断言 —— 如果不实现，文件无法编译通过。
var _ schema.TablerWithNamer = Token{}
