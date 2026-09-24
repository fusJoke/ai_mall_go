// Package registry 收集并下发路由挂载点。
//
// 设计要点：
//   - 子路由文件的 init() 通过 Register(prefix, method, path, handler) 声明路由，
//     不直接持有 *gin.Engine —— 因为 main() 还没跑，引擎未建。
//   - router.Setup(engine) 在启动期调 Apply(engine)，把所有挂载一次性下发到引擎：
//     每个独立 prefix 懒创建一次 r.Group，同 prefix 共享同一组。
//   - 新增子路由 = 新建一个文件 + init() 调 Register，router.go 加一行空白导入即可，
//     无需修改 Apply / Setup。
package registry

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// Mount 是单条延迟挂载：被挂到 prefix 标识的 RouterGroup 下。
type Mount struct {
	Prefix  string // "" 表示根，"/admin" "/user" 等表示对应子路由组
	Method  string // HTTP 方法，"GET" "POST" ...
	Path    string // 组内路径（不带 prefix），如 "/ping"
	Handler gin.HandlerFunc
}

var mounts []Mount

// Register 声明一条路由挂载。一般在子路由文件的 init() 中调用。
//
// Setup 之后再调 Register 是 no-op 之外的编程错误：Apply 已经把 mounts 置空，
// 后续调用会让那条路由永远挂不上去。建议在测试里加守卫，
// 或者保持"init 期声明、main 期下发"的纪律。
func Register(prefix, method, path string, handler gin.HandlerFunc) {
	mounts = append(mounts, Mount{
		Prefix:  prefix,
		Method:  method,
		Path:    path,
		Handler: handler,
	})
}

// Apply 把所有挂载下发到 gin 引擎。每个独立 prefix 创建一次 r.Group，
// 同 prefix 的多条挂载共享同一组。
//
// 由 router.Setup 调用，且只应被调一次 —— 重复调用会因为 mounts 已被清空而变成 no-op。
func Apply(r *gin.Engine) {
	groups := map[string]*gin.RouterGroup{
		"": &r.RouterGroup,
	}

	// 第一遍：保证每个出现过的 prefix 都有对应 group。
	for _, m := range mounts {
		if _, ok := groups[m.Prefix]; !ok {
			groups[m.Prefix] = r.Group(m.Prefix)
		}
	}

	// 第二遍：按 group 把挂载挂到引擎上。
	for _, m := range mounts {
		g, ok := groups[m.Prefix]
		if !ok {
			// 上一遍已经保证存在，这里理论上走不到。
			panic(fmt.Sprintf("router/registry: missing group for prefix %q", m.Prefix))
		}
		g.Handle(m.Method, m.Path, m.Handler)
	}

	// 清空挂载表：避免后续误调 Register 时旧挂载被重复挂载。
	mounts = nil
}

// Pending 返回尚未下发的挂载数。仅用于测试 / 调试，不应在生产逻辑里调用。
func Pending() int {
	return len(mounts)
}
