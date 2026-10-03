package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/model"
	"ai-go-mall/internal/model/mall"
	adminRepo "ai-go-mall/internal/repository/admin"
	supplierRepo "ai-go-mall/internal/repository/supplier"
	userRepo "ai-go-mall/internal/repository/user"
)

// =============================================================================
// 三段式鉴权：AdminAuth / UserAuth / SupplierAuth
// =============================================================================
//
// 共同点：
//   - Bearer token 解析（RFC 6750 scheme 大小写不敏感）
//   - 调 checkToken 校验 token（过期 / 不存在 / 错类型 → 401）
//   - 按 token type 路由：type="admin" → AdminAuth；type="user" → UserAuth；type="supplier" → SupplierAuth
//   - token 校验通过后查 entity 写入 gin context，handler 走 *FromContext 拿
//
// 差异点：
//   - tokenType 期望值（admin/user/supplier）
//   - lookup 函数（admin → AdminRepository；user → UserRepository；supplier → SupplierUserRepository）
//   - context key（adminContextKey / userContextKey / supplierContextKey）+ *FromContext 助手
//
// 设计动机：
//   - token 表只存 user_id（int64）+ type（varchar），不带角色/权限字段；
//     entity 加载在中间件层完成，避免 handler 再查一次。
//   - 三段式拆开而非泛型抽象：lookup 返回值类型不同（Admin / User / MallSupplierUser），
//     Go 泛型在此处要么 trade-off 类型擦除要么写出冗余样板；按业务三段式更清晰。

// checkToken 是 AdminAuth / UserAuth / SupplierAuth 实际调用的 token 校验入口。
//
// 生产实现：转发到 token.Get().Check；
// 测试可整体替换为 fake（auth_test.go）。替换时记得 defer 还原，
// 避免污染其它并行测试。
//
// 用函数变量而非 interface：token.Manager 是具体类型，抽 interface
// 会扩散到 infra 包与运行时影响范围。function hook 局限在 middleware
// 包内，副作用最小。
var checkToken = func(ctx context.Context, rawToken string) (*model.Token, error) {
	mgr := token.Get()
	if mgr == nil {
		return nil, errTokenManagerUnavailable
	}
	return mgr.Check(ctx, rawToken)
}

// lookupAdmin / lookupUser / lookupSupplierUser 是 token 校验通过后执行的
// entity 查询入口。生产实现走 database.Get() → repo.GetByID；
// 测试可整体替换为 fake。
//
// 行为约定（三个 lookup 共享）：
//   - database.Get() 未初始化（测试场景）：返回 (nil, nil)，不 panic；
//   - entity 不存在 / 已软删：返回 (nil, nil)；
//   - DB 错误：返回 (nil, err)。
//
// 中间件拿到 (nil, err) 时映射成 500 auth.internal；拿到 (nil, nil) 时
// 不 abort（token 已合法，handler 自决），只把 nil 塞进 context。
var (
	lookupAdmin = func(c *gin.Context, uid uint) (*model.Admin, error) {
		db := database.Get()
		if db == nil {
			return nil, nil
		}
		return adminRepo.NewAdminRepository(db).GetByID(uid)
	}

	// lookupUser / lookupSupplierUser 同 lookupAdmin 模式：db==nil 时返回
	// (nil, nil)（middleware 不 abort，handler 自决）；db 非 nil 时按 uid 取行。
	//
	// 与 lookupAdmin 的差异：admin 仓储通过 adminRepo.NewAdminRepository(db)
	// 显式收 db 入参；user / supplier user 仓储走 NewRepository()（无参）内部用
	// repository.DB(c) 取请求作用域 db。这里 c 是 middleware 持有的 *gin.Context，
	// repository.DB(c) 通过 DBMiddleware 注入；与 admin 仓储的"显式收 db"是
	// 同构的两种风格，不影响业务行为。
	lookupUser = func(c *gin.Context, uid uint) (*model.User, error) {
		db := database.Get()
		if db == nil {
			return nil, nil
		}
		return userRepo.NewRepository().GetByID(c, int64(uid))
	}

	lookupSupplierUser = func(c *gin.Context, uid uint) (*mall.MallSupplierUser, error) {
		db := database.Get()
		if db == nil {
			return nil, nil
		}
		return supplierRepo.NewSupplierUserRepository().GetByID(c, int64(uid))
	}
)

// errTokenManagerUnavailable：token infra 未初始化时返回的哨兵；
// 中间件把它映射成 500 auth.internal 而非 401。
var errTokenManagerUnavailable = errors.New("token manager unavailable")

// context keys：当前请求上下文写入 entity 的键名。
//
// handler 通过 *FromContext(c) 读取；值为对应 entity 类型指针，可能为 nil。
const (
	adminContextKey    = "admin.current"
	userContextKey     = "user.current"
	supplierContextKey = "supplier.current"
)

// AdminFromContext 返回当前请求上下文中由 AdminAuth 写入的管理员记录。
//
// 可能返回 nil —— 通常发生在：admin 被并发删除 / token 签发时 admin 存在、
// 请求到达前已被删。调用方拿到 nil 时按业务自行处理（init handler 走 403）。
func AdminFromContext(c *gin.Context) *model.Admin {
	return getTypedFromContext[*model.Admin](c, adminContextKey)
}

// UserFromContext 返回当前请求上下文中由 UserAuth 写入的会员记录。
//
// 可能返回 nil（同 AdminFromContext 的语义）。
func UserFromContext(c *gin.Context) *model.User {
	return getTypedFromContext[*model.User](c, userContextKey)
}

// SupplierFromContext 返回当前请求上下文中由 SupplierAuth 写入的供应商账号记录。
//
// 可能返回 nil（同 AdminFromContext 的语义）。
func SupplierFromContext(c *gin.Context) *mall.MallSupplierUser {
	return getTypedFromContext[*mall.MallSupplierUser](c, supplierContextKey)
}

// getTypedFromContext 是 *FromContext 的泛型实现：从 c.Get(key) 拿值并做类型断言。
// 抽出来避免三个 *FromContext 函数复制粘贴。
func getTypedFromContext[T any](c *gin.Context, key string) T {
	var zero T
	v, ok := c.Get(key)
	if !ok || v == nil {
		return zero
	}
	t, _ := v.(T)
	return t
}

// tokenTypeXxx 是三个中间件各自接受的 token 类型常量。
//
// 防止后续签发新类型 token 后，被无意用于鉴权其它路由。
const (
	tokenTypeAdmin    = "admin"
	tokenTypeUser     = "user"
	tokenTypeSupplier = "supplier"
)

// AdminAuth 返回要求合法 admin Bearer token 的 gin 中间件。
//
// 解析规则（RFC 6750）：Authorization 头的 scheme 部分大小写不敏感；
// "Bearer"、"bearer"、"BEARER" 都接受。后接一个 token 字符串（TrimSpace 容忍尾随空白）。
// 其他形式 —— 空 header / 非 Bearer scheme / 截取后为空 —— 一律 401（auth.missing_token）。
//
// 校验规则：调 checkToken（生产 = token.Manager.Check）：
//   - 不存在（ErrTokenNotFound）      → 401 auth.token_not_found
//   - 已过期（ErrTokenExpired）        → 401 auth.token_expired
//   - 类型不符（非 tokenTypeAdmin）    → 401 auth.token_type_mismatch
//   - 其他错误                          → 401 auth.invalid_token
//   - token infra 未初始化             → 500 auth.internal（fail-closed）
//
// token 校验通过后调 lookupAdmin 拿当前 admin 写入 context，供 handler 复用：
//   - DB 错误                           → 500 auth.internal
//   - admin 不存在 / 已软删             → 不 abort，nil context，handler 自决
//   - admin 存在                        → c.Set(adminContextKey, *model.Admin)
//
// 错误响应只回固定文案，不透传 err.Error() 给客户端 —— 防止「token: not found」
// 等驱动层内部措辞泄露到用户面前。
func AdminAuth() gin.HandlerFunc {
	return authMiddleware(tokenTypeAdmin, func(c *gin.Context, uid uint) (any, error) {
		return lookupAdmin(c, uid)
	}, adminContextKey)
}

// UserAuth 返回要求合法 user Bearer token 的 gin 中间件。
//
// 行为同 AdminAuth，仅差异在 token 类型（type="user"）与 entity 加载
// （userRepo.GetByID → *model.User 写入 context）。
//
// handler 通过 UserFromContext(c) 取当前会员记录。
func UserAuth() gin.HandlerFunc {
	return authMiddleware(tokenTypeUser, func(c *gin.Context, uid uint) (any, error) {
		return lookupUser(c, uid)
	}, userContextKey)
}

// SupplierAuth 返回要求合法 supplier Bearer token 的 gin 中间件。
//
// 行为同 AdminAuth，仅差异在 token 类型（type="supplier"）与 entity 加载
// （supplierRepo.NewSupplierUserRepository.GetByID → *mall.MallSupplierUser 写入 context）。
//
// handler 通过 SupplierFromContext(c) 取当前供应商账号记录。
func SupplierAuth() gin.HandlerFunc {
	return authMiddleware(tokenTypeSupplier, func(c *gin.Context, uid uint) (any, error) {
		return lookupSupplierUser(c, uid)
	}, supplierContextKey)
}

// authMiddleware 是三段式鉴权的共同实现：
//   1. 解析 Authorization 头（Bearer scheme，大小写不敏感）；
//   2. 调 checkToken 校验 token（401 各种 token 错误 / 500 token infra 未初始化）；
//   3. 校验 token.Type == expectedType（401 auth.token_type_mismatch）；
//   4. 调 lookup 拿 entity（500 auth.internal / nil context）；
//   5. 把 entity（非 nil 时）写入 ctxKey。
//
// 三个对外函数（AdminAuth / UserAuth / SupplierAuth）的差异完全由 expectedType /
// lookup / ctxKey 三个参数承载；中间件逻辑共用，零分支复制。
func authMiddleware(expectedType string, lookup func(c *gin.Context, uid uint) (any, error), ctxKey string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authz := c.GetHeader("Authorization")
		// RFC 6750: scheme 部分大小写不敏感。
		if len(authz) < len(bearerPrefix) || !strings.EqualFold(authz[:len(bearerPrefix)], bearerPrefix) {
			abort401(c, "auth.missing_token", msgMissingToken)
			return
		}
		rawToken := strings.TrimSpace(authz[len(bearerPrefix):])
		if rawToken == "" {
			abort401(c, "auth.missing_token", msgEmptyBearer)
			return
		}

		tok, err := checkToken(c.Request.Context(), rawToken)
		if err != nil {
			if errors.Is(err, errTokenManagerUnavailable) {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
					"code":    "auth.internal",
					"message": msgAuthInternal,
				})
				return
			}
			code, msg := classifyTokenErr(err)
			abort401(c, code, msg)
			return
		}
		if tok == nil || tok.Type != expectedType {
			// 命中此分支说明 token 存在但类型不对（如 user supplier 用 admin），
			// 等同「不是合法的 X token」。
			abort401(c, "auth.token_type_mismatch", msgTypeMismatch)
			return
		}

		// token 已合法 —— 把当前 entity 写入 context 供下游 handler 使用。
		// tok.UserID 是 int64（model.Token），repo.GetByID 收 uint，
		// 这里强转；实际系统中 ID 始终为正，转 uint 不会失真。
		entity, lookupErr := lookup(c, uint(tok.UserID))
		if lookupErr != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"code":    "auth.internal",
				"message": msgAuthInternal,
			})
			return
		}
		if entity != nil {
			c.Set(ctxKey, entity)
		}

		c.Next()
	}
}

// bearerPrefix 是 RFC 6750 Bearer scheme + 分隔空格。
const bearerPrefix = "Bearer "

// 客户端可见的固定文案（不暴露驱动层细节、可本地化）。
const (
	msgMissingToken  = "missing or malformed Authorization header"
	msgEmptyBearer   = "empty bearer token"
	msgTokenNotFound = "token not found"
	msgTokenExpired  = "token expired"
	msgTypeMismatch  = "token type not allowed for this endpoint"
	msgInvalidToken  = "invalid token"
	msgAuthInternal  = "auth subsystem unavailable"
)

// classifyTokenErr 把 checkToken 返回的 error 映射成 (code, message)。
//
// message 故意用固定文案而非 err.Error()：驱动层 / 哨兵的 error 文本会暴露
// 实现细节（如「token: not found」「token: expired」），对客户端无用且易被
// 攻击者用作 oracle。
func classifyTokenErr(err error) (code, message string) {
	switch {
	case errors.Is(err, token.ErrTokenExpired):
		return "auth.token_expired", msgTokenExpired
	case errors.Is(err, token.ErrTokenNotFound):
		return "auth.token_not_found", msgTokenNotFound
	default:
		return "auth.invalid_token", msgInvalidToken
	}
}

// abort401 收敛 401 响应形状（避免在多个分支重复写信封结构）。
func abort401(c *gin.Context, code, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
		"code":    code,
		"message": message,
	})
}

// 包级 io 兜底，确保文件至少有 1 个 import（rate_limit 引入 http 后这里不需要额外 import）。
var _ = http.StatusOK