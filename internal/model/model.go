// Package model 存放与数据表一一对应的结构体。
//
// 本包不放业务逻辑、不放 SQL，每个表结构体最多再加一个 TableName() 方法。
// 所有模型通过 init() 自注册到包内注册表，由 internal/infra/database 在
// AutoMigrate 时统一拉取，因此新增模型不需要修改迁移代码。
//
// 典型写法见 user.go（单个模型）：
//
//	func init() {
//	    model.Register(User{})
//	}
//
// 也支持一次性注册多个：
//
//	func init() {
//	    model.Register(User{}, Product{}, Order{})
//	}
package model

import (
	"reflect"
	"sync"
)

// 注册表。模型通过 init() 自注册；database.Init() 通过 All() 一次性取出。
//
// 互斥锁用于兜底运行期可能存在的并发注册；启动期 init() 串行执行，理论上不触发。
var (
	mu  sync.Mutex
	all []any
)

// Register 注册一个或多个模型用于 AutoMigrate。一般在模型文件自身的 init() 中调用。
//
// 用法与 db.AutoMigrate 对齐，可以一次挂多个：
//
//	Register(User{}, Product{}, Order{})
//
// 重复注册相同类型是 no-op（按指针 / 值的底层类型去重），避免误把同一个模型
// 挂两遍导致 AutoMigrate 重复建表。nil 实参会被跳过。
func Register(models ...any) {
	mu.Lock()
	defer mu.Unlock()

	for _, m := range models {
		t := typeOf(m)
		if t == nil {
			continue
		}

		duplicated := false
		for _, existing := range all {
			if typeOf(existing) == t {
				duplicated = true
				break
			}
		}
		if !duplicated {
			all = append(all, m)
		}
	}
}

// All 返回当前已注册的全部模型，按注册顺序返回副本。
//
// 调用方一般是 internal/infra/database.Init() 内的 AutoMigrate。
// 返回副本，避免外部修改 runtime slice 污染注册表。
func All() []any {
	mu.Lock()
	defer mu.Unlock()
	out := make([]any, len(all))
	copy(out, all)
	return out
}

// Reset 清空已注册的模型。仅供测试使用。
func Reset() {
	mu.Lock()
	defer mu.Unlock()
	all = nil
}

// typeOf 取指针 / 接口底下的具体类型。nil 接口直接返回 nil。
func typeOf(m any) reflect.Type {
	if m == nil {
		return nil
	}
	t := reflect.TypeOf(m)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	return t
}
