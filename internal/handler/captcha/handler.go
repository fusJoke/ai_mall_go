// Package captcha 是点选验证码的 HTTP 处理层。
//
// 路由：
//   - GET  /common/captcha/create  → Handler.CreateClick
//   - POST /common/captcha/verify  → Handler.VerifyClick
//
// 错误码映射（按 design.md D8）：
//
//	infra error     HTTP  业务错误码
//	---------       ----  -----------
//	ErrInvalidInput  400   captcha.invalid_input
//	ErrNotFound      404   captcha.not_found
//	ErrExpired       410   captcha.expired
//	ErrMismatch      401   captcha.mismatch
//	其他             500   captcha.internal
package captcha

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	captchaInfra "ai-go-mall/internal/infra/captcha"
	captchaSvc "ai-go-mall/internal/service/captcha"
)

// Handler 是点选验证码 HTTP 处理器，持有 service.Service。
type Handler struct {
	svc captchaSvc.Service
}

// NewHandler 接收 service，返回 *Handler。
func NewHandler(svc captchaSvc.Service) *Handler {
	return &Handler{svc: svc}
}

// CreateClickResp 是 GET /common/captcha/create 的成功响应。
type CreateClickResp struct {
	Key      string   `json:"key"`
	Elements []string `json:"elements"`
	Image    string   `json:"image"`
	Width    int      `json:"width"`
	Height   int      `json:"height"`
}

// CreateClick 处理 GET /common/captcha/create。
//
// @Summary      生成点选验证码
// @Description  服务端生成一道点选验证码；返回的 key 须先经 /common/captcha/verify 预校验坐标精度，才能用于业务（如登录）。
// @Tags         captcha
// @Produce      json
// @Success      200  {object}  captcha.CreateClickResp  "成功：返回 key / elements / image 等"
// @Failure      500  {object}  map[string]string        "captcha.internal"
// @Router       /common/captcha/create [get]
func (h *Handler) CreateClick(c *gin.Context) {
	got, err := h.svc.CreateClick(c.Request.Context())
	if err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, CreateClickResp{
		Key:      got.Key,
		Elements: got.Elements,
		Image:    got.Image,
		Width:    got.Width,
		Height:   got.Height,
	})
}

// VerifyClickReq 是 POST /common/captcha/verify 的请求体；JSON tag 全小写。
type VerifyClickReq struct {
	Key    string  `json:"key" binding:"required"`
	Points []Point `json:"points" binding:"required"`
	W      int     `json:"w" binding:"required"`
	H      int     `json:"h" binding:"required"`
}

// Point 是用户点选坐标（图片原始像素坐标系）。
type Point struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// VerifyClick 处理 POST /common/captcha/verify。
//
// 该端点固定为「预检」语义：仅校验精度，不消耗 key（deleteOnSuccess=false）。
//
// @Summary      校验点选坐标精度
// @Description  仅校验坐标精度，不消耗 key（deleteOnSuccess=false）。
// @Tags         captcha
// @Accept       json
// @Produce      json
// @Param        req  body      captcha.VerifyClickReq   true  "点选坐标 + 验证码 key"
// @Success      200  {object}  map[string]interface{}   "成功：{ ok: true }"
// @Failure      400  {object}  map[string]string        "captcha.invalid_input"
// @Failure      401  {object}  map[string]string        "captcha.mismatch"
// @Failure      404  {object}  map[string]string        "captcha.not_found"
// @Failure      410  {object}  map[string]string        "captcha.expired"
// @Failure      500  {object}  map[string]string        "captcha.internal"
// @Router       /common/captcha/verify [post]
func (h *Handler) VerifyClick(c *gin.Context) {
	var req VerifyClickReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "captcha.invalid_input",
			"message": err.Error(),
		})
		return
	}

	infraReq := &captchaInfra.VerifyReq{
		Key:    req.Key,
		W:      req.W,
		H:      req.H,
		Points: make([]captchaInfra.Point, len(req.Points)),
	}
	for i, p := range req.Points {
		infraReq.Points[i] = captchaInfra.Point{X: p.X, Y: p.Y}
	}

	if err := h.svc.VerifyClick(c.Request.Context(), infraReq); err != nil {
		writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// writeError 按 design.md D8 表把 infra sentinel error 映射为 HTTP 状态 + 业务错误码。
func writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, captchaInfra.ErrInvalidInput):
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "captcha.invalid_input",
			"message": err.Error(),
		})
	case errors.Is(err, captchaInfra.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{
			"code":    "captcha.not_found",
			"message": err.Error(),
		})
	case errors.Is(err, captchaInfra.ErrExpired):
		c.JSON(http.StatusGone, gin.H{
			"code":    "captcha.expired",
			"message": err.Error(),
		})
	case errors.Is(err, captchaInfra.ErrMismatch):
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    "captcha.mismatch",
			"message": err.Error(),
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "captcha.internal",
			"message": err.Error(),
		})
	}
}
