package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/token"
	"ai-go-mall/internal/model"
)

// checkToken 是 AdminAuth 实际调用的 token 校验入口。
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

// errTokenManagerUnavailable：token infra 未初始化时返回的哨兵；
// 中间件把它映射成 500 auth.internal 而非 401。
var errTokenManagerUnavailable = errors.New("token manager unavailable")

// tokenTypeAdmin 是本中间件接受的 token 类型：仅允许 type == "admin" 的 token 通过。
//
// 防止后续签发 user/refresh/api 等其他类型 token 后，被无意用于鉴权 admin 路由。
const tokenTypeAdmin = "admin"

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
// 错误响应只回固定文案，不透传 err.Error() 给客户端 —— 防止「token: not found」
// 等驱动层内部措辞泄露到用户面前。
func AdminAuth() gin.HandlerFunc {
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
		if tok == nil || tok.Type != tokenTypeAdmin {
			// 命中此分支说明 token 存在但类型不对（user/refresh/api ...），
			// 等同「不是合法的 admin token」。
			abort401(c, "auth.token_type_mismatch", msgTypeMismatch)
			return
		}

		c.Next()
	}
}

// bearerPrefix 是 RFC 6750 Bearer scheme + 分隔空格。
const bearerPrefix = "Bearer "

// 客户端可见的固定文案（不暴露驱动层细节、可本地化）。
const (
	msgMissingToken    = "missing or malformed Authorization header"
	msgEmptyBearer     = "empty bearer token"
	msgTokenNotFound   = "token not found"
	msgTokenExpired    = "token expired"
	msgTypeMismatch    = "token type not allowed for this endpoint"
	msgInvalidToken    = "invalid token"
	msgAuthInternal    = "auth subsystem unavailable"
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