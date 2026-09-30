package model

import (
	"reflect"
	"testing"
)

type fakeModel struct {
	ID int64
}

func (fakeModel) TableName() string { return "fake_models" }

func init() {
	Register(fakeModel{})
}

func TestRegister_AndAll(t *testing.T) {
	// fakeModel 已经由包内 init() 注册；User 也是。
	all := All()

	got := map[reflect.Type]bool{}
	for _, m := range all {
		got[typeOf(m)] = true
	}

	if !got[reflect.TypeOf(User{})] {
		t.Errorf("expected User to be registered, got %v", all)
	}
	if !got[reflect.TypeOf(fakeModel{})] {
		t.Errorf("expected fakeModel to be registered, got %v", all)
	}
	if !got[reflect.TypeOf(Admin{})] {
		t.Errorf("expected Admin to be registered, got %v", all)
	}
	if !got[reflect.TypeOf(AdminRule{})] {
		t.Errorf("expected AdminRule to be registered, got %v", all)
	}
	if !got[reflect.TypeOf(AdminGroup{})] {
		t.Errorf("expected AdminGroup to be registered, got %v", all)
	}
	if !got[reflect.TypeOf(AdminGroupAccess{})] {
		t.Errorf("expected AdminGroupAccess to be registered, got %v", all)
	}
}

func TestRegister_DedupByType(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	// 单次调用里既有值又有重复 —— 全部去重。
	Register(User{}, &User{}, User{}) // 指针版本应被识别为同一类型

	if got := len(All()); got != 1 {
		t.Fatalf("expected 1 registered model after dedup, got %d", got)
	}
}

func TestRegister_Variadic(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	Register(User{}, fakeModel{})

	if got := len(All()); got != 2 {
		t.Fatalf("expected 2 registered models, got %d", got)
	}
}

func TestRegister_NilIgnored(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	// nil 出现在变长参数任意位置都应被跳过，其他实参照常注册。
	Register(nil, User{}, nil)

	all := All()
	if got := len(all); got != 1 {
		t.Fatalf("expected 1 registered model (nil skipped), got %d", got)
	}
	if _, ok := all[0].(User); !ok {
		t.Fatalf("expected remaining entry to be User, got %T", all[0])
	}
}

func TestAll_ReturnsCopy(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	Register(User{})
	a := All()
	a[0] = "tampered"

	b := All()
	if _, ok := b[0].(User); !ok {
		t.Fatalf("expected All() to return a defensive copy; original entry mutated to %T", b[0])
	}
}
