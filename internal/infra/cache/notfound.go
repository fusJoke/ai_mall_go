// notfound.go — 缓存防穿透的统一 notFound 语义（D5.1 / 任务 17.4）。
//
// 三层缓存共用一套「数据不存在」的表达：
//
//	DB/loader 返回 ErrNotFound（IsNotFound 判定）
//	  → L1/L2 写 {"_notFound":true} 占位（NotFoundPlaceholderTTL 短过期）
//	  → Hotspot 写 notFound 信封（NotFoundPayload 带逻辑时间戳，随 staleAfter 刷新）
//	  → 后续相同 key 的读直接得到「不存在」，不再打 DB
//
// 写路径（业务创建数据）必须主动失效占位：Invalidate 会 DEL 同一个 key，
// 占位与真实数据共存于同一 key 空间，无需额外清理逻辑。
package cache

import (
	"errors"
	"time"
)

// ErrNotFound 表达「业务数据不存在」——与 ErrCacheMiss（缓存层本身没这个 key）
// 严格区分：ErrCacheMiss 是缓存状态，ErrNotFound 是业务事实。
//
// loader / service 用它（或 wrap 它的业务 sentinel，如 ErrBlindBoxNotFound）
// 通知缓存层写占位。
var ErrNotFound = errors.New("cache: not found")

// NotFoundPlaceholderTTL 是 L1/L2 占位的存活时长（D5.1：30s 平衡穿透防护
// 与「数据后来才创建」的可见性窗口）。
const NotFoundPlaceholderTTL = 30 * time.Second

// notFoundValueJSON 是 L1/L2 层的占位值（JSON 标记）。
const notFoundValueJSON = `{"_notFound":true}`

// NotFoundPlaceholderValue 返回 L1/L2 层占位值。
func NotFoundPlaceholderValue() string { return notFoundValueJSON }

// IsNotFoundValue 判断 L1/L2 层读到的值是否是占位。
func IsNotFoundValue(val string) bool { return val == notFoundValueJSON }

// IsNotFound 判断错误链上是否有 ErrNotFound。
//
// 业务 sentinel 通过 %w 包装 ErrNotFound（如 ErrBlindBoxNotFound），
// 使 handler 的 errors.Is(err, ErrBlindBoxNotFound) 与缓存层的
// IsNotFound(err) 同时成立。
func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) }

// NotFoundPayload 是 Hotspot 层的占位载荷（任务 17.3：带逻辑时间戳，
// 存进热点信封后永驻，随 staleAfter 逻辑过期触发重查——查到真实数据即替换）。
type NotFoundPayload struct {
	RefreshedAt time.Time `json:"refreshed_at"`
}

// WrapNotFound 构造 Hotspot 层占位载荷（val 参数为未来扩展保留：
// 可携带"不存在的原因"等上下文；当前忽略）。
func WrapNotFound(val any) any {
	return &NotFoundPayload{RefreshedAt: time.Now()}
}

// UnwrapNotFound 判断热点层解码出的值是否是占位载荷。
func UnwrapNotFound(val any) (*NotFoundPayload, bool) {
	p, ok := val.(*NotFoundPayload)
	if !ok || p == nil {
		return nil, false
	}
	return p, true
}
