// Package token 提供 token（API 调用凭证）的生成、查询、校验与清理能力。
//
// 设计：
//   - 底层支持多驱动（database / redis ...），驱动放 driver/ 子目录；
//     每个驱动一个文件，文件名即驱动名。
//   - Manager 持有 Driver，对外暴露统一的 token 操作接口；
//     额外提供 Check 方法（带过期判断的高频校验），由 Manager 组合 Get 实现，
//     这样 driver 不需要感知"过期"概念，符合单一职责。
//   - token 入库使用 SHA256(hex) 哈希，明文不会落库。
//
// 启动流程：cmd/serve/main.go 在 database.Init() 之后调用 token.Init()，
// 内部读取 config.Get().Token.Driver 决定加载哪个驱动。
package token

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ai-go-mall/internal/infra/config"
	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/token/driver"
	"ai-go-mall/internal/model"
)

// Sentinel errors：调用方通过 errors.Is 区分业务分支。
//
// 这些变量是对 driver 包同名 error 的 re-export，避免 driver → token 反向依赖。
// 真实定义在 driver 包（因为底层行为是 driver 实现的），
// 这里再 var 别名导出，让调用方统一用 token.ErrXxx。
var (
	// ErrTokenNotFound token 不存在（哈希后查不到任何记录）。
	ErrTokenNotFound = driver.ErrTokenNotFound

	// ErrTokenInvalid token 入参非法（空字符串、nil 对象等）。
	ErrTokenInvalid = driver.ErrTokenInvalid

	// ErrTokenExpired token 已过期（Get 成功后 ExpiresAt 早于当前时间）。
	// 这是 Manager.Check 独有的业务错误，由 token 包自身定义。
	ErrTokenExpired = errors.New("token: expired")
)

// Driver 是 token 存储后端的统一接口。
//
// 4 个方法对应 token 的 4 类操作。所有 driver 必须以"明文 token 入参、
// 内部 SHA256 后再访问存储"为契约 —— 任何 driver 都不能保存明文。
type Driver interface {
	// Create 把 t 写入存储。调用前需保证 t.Token 是明文；驱动内部负责哈希。
	Create(ctx context.Context, t *model.Token) error

	// Get 按明文 token 读取一条记录；不存在返回 ErrTokenNotFound。
	Get(ctx context.Context, rawToken string) (*model.Token, error)

	// Delete 按明文 token 软删一条记录；不存在不算错（no-op）。
	Delete(ctx context.Context, rawToken string) error

	// Clear 删除指定用户在指定类型下的全部 token；用于"挤下线" / "刷新登录态"场景。
	Clear(ctx context.Context, userID int64, tokenType string) error
}

// Manager 是 token 业务的对外门面，持有 Driver 提供基础 CRUD，
// 并在之上叠加过期判断等业务规则。
type Manager struct {
	driver Driver
}

// mgr 是 Init 缓存的 Manager 单例。Init 未调用或失败时为 nil。
var mgr *Manager

// Init 读取 config.Get().Token.Driver，按配置实例化对应 driver，
// 包装成 Manager 缓存到包级变量。
//
// 同一进程重复调用是 no-op；如需强制重载，调用 Reset 后再调用 Init。
//
// 注意：依赖 database.Get()，必须先调 database.Init()。
func Init() error {
	if mgr != nil {
		return nil
	}

	driverName := config.Get().Token.Driver
	d, err := newDriver(driverName)
	if err != nil {
		return err
	}

	mgr = &Manager{driver: d}
	return nil
}

// Get 返回已初始化的 *Manager。Init 未调用过或失败时返回 nil。
func Get() *Manager {
	return mgr
}

// Reset 清空 Manager 缓存。专供测试使用。
func Reset() {
	mgr = nil
}

// newDriver 是 driver 工厂：根据 driver 名字返回对应实例。
// 后续新增 driver（redis / memcached）只需在此追加 case。
func newDriver(name string) (Driver, error) {
	switch name {
	case "database", "":
		return driver.NewDatabase(database.Get()), nil
	default:
		return nil, fmt.Errorf("token: unknown driver %q", name)
	}
}

// --- Manager 转发方法（参数校验后转 driver） ---

// Create 把 t 写入存储。t.Token 应为明文；driver 内部负责哈希。
func (m *Manager) Create(ctx context.Context, t *model.Token) error {
	if t == nil {
		return ErrTokenInvalid
	}
	return m.driver.Create(ctx, t)
}

// Get 按明文 token 读取一条记录；不存在返回 ErrTokenNotFound。
func (m *Manager) Get(ctx context.Context, rawToken string) (*model.Token, error) {
	if rawToken == "" {
		return nil, ErrTokenInvalid
	}
	return m.driver.Get(ctx, rawToken)
}

// Delete 按明文 token 软删一条记录；不存在 no-op。
func (m *Manager) Delete(ctx context.Context, rawToken string) error {
	if rawToken == "" {
		return ErrTokenInvalid
	}
	return m.driver.Delete(ctx, rawToken)
}

// Clear 删除指定用户在指定类型下的全部 token；用于"挤下线" / "重置登录态"。
func (m *Manager) Clear(ctx context.Context, userID int64, tokenType string) error {
	if userID <= 0 || tokenType == "" {
		return ErrTokenInvalid
	}
	return m.driver.Clear(ctx, userID, tokenType)
}

// --- Manager 业务方法（不在 Driver 接口里） ---

// Check 校验 token 是否有效：先 Get 出记录，再判断 ExpiresAt 是否早于当前时间。
//
// 过期返回 (nil, ErrTokenExpired)；不存在返回 (nil, ErrTokenNotFound)；
// 都用 sentinel error，调用方可用 errors.Is 区分。
func (m *Manager) Check(ctx context.Context, rawToken string) (*model.Token, error) {
	t, err := m.Get(ctx, rawToken)
	if err != nil {
		return nil, err
	}
	if t.ExpiresAt.Before(time.Now()) {
		return nil, ErrTokenExpired
	}
	return t, nil
}
