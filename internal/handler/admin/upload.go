package admin

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/upload"
)

// UploadService 是 infra/upload.Service 的最小接口。
//
// 让 handler 在测试时可注入 mock；router/admin 装配时传 upload.Get()。
// *upload.Service 自然满足此接口（方法签名一致），无需 adapter。
type UploadService interface {
	Upload(in *upload.UploadInput) (*upload.UploadResult, error)
}

// UploadHandler 处理 POST /admin/ajax/upload。
//
// 接收 multipart/form-data 中的 file 字段，按 query ?topic=xxx 分类存储
// （缺省 admin），返回 driver 拼出的对外 URL。
//
// 设计要点：
//   - 不持有 *upload.Service 具体类型 —— 通过 UploadService 接口注入，
//     避免 handler 与 infra 包强耦合、便于单测 mock。
//   - 错误响应固定文案，不暴露 err.Error()（驱动层细节可能含路径 / SQL）。
//   - 成功响应遵循项目 {code, message, data} 信封；code = 0 触发前端
//     response interceptor 自动解包，data.file.url 即前端 v-model 写入值。
type UploadHandler struct {
	svc UploadService
}

// NewUploadHandler 构造 UploadHandler。
func NewUploadHandler(svc UploadService) *UploadHandler {
	return &UploadHandler{svc: svc}
}

// uploadResponse 是 /admin/ajax/upload 成功时的 data 载荷。
//
// 顶层信封 {code, message, data} 由 handler 在 c.JSON 时直接拼；这里只描述
// data 部分。前端 fileUpload 调用方解包后访问 res.data.file.url。
type uploadResponse struct {
	File uploadFile `json:"file"`
}

// uploadFile 描述一次成功上传的对外可见信息。
type uploadFile struct {
	// URL 是 driver.Url() 返回的对外可访问地址（如 /uploads/avatar/.../x.jpg）。
	// 已经是 URL-encoded 的相对路径；前端组件若需绝对地址，再调 fullUrl() 包一层。
	URL string `json:"url"`
	// StoredPath 是 format 模板渲染后的相对路径（如 avatar/20260930/x.jpg）。
	StoredPath string `json:"stored_path"`
	// Size 是实际写入字节数。
	Size int64 `json:"size"`
	// Suffix 是小写后缀（不含前导点，如 "jpg"）。
	Suffix string `json:"suffix"`
}

// Upload 处理 multipart/form-data POST /admin/ajax/upload。
//
// 数据流：
//  1. 从 form 拿 "file" 字段；缺失 / 不可读 → 400 upload.invalid_input
//  2. topic 取自 query "topic"，缺省 "admin"
//  3. 调 svc.Upload；按 sentinel error 分类映射 HTTP + 文案
//  4. 成功 → 200 + {code: 0, message: "ok", data: {file: {...}}}
//
// 错误映射：
//   - ErrInvalidInput  → 400 upload.invalid_input
//   - ErrInvalidSuffix → 400 upload.invalid_suffix
//   - ErrFileTooLarge  → 413 upload.file_too_large
//   - 其他（含 ErrDriverFailure 包装的写盘错误）→ 500 upload.internal
//
// 注意：multipart 自身的内存上限（gin.DefaultMaxMultipartMemory = 32MB）会在
// 我们的 svc.Upload 之前生效；超大文件会被 gin 直接拒绝（HTTP 413），不走 svc。
// 我们额外在 svc 层再做一次 LimitReader 校验作为兜底。
//
// @Summary      管理员上传文件
// @Description  接收 multipart/form-data 中的 file 字段，按 ?topic=xxx 分类落到 infra/upload 配置的存储后端；返回 driver 拼出的对外 URL。
// @Tags         admin
// @Accept       multipart/form-data
// @Produce      json
// @Security     BearerAuth
// @Param        file  formData  file    true  "上传的文件"
// @Param        topic query    string  false "业务分类；缺省 admin"
// @Success      200   {object} admin.uploadResponse "upload.ok"
// @Failure      400   {object} map[string]string    "upload.invalid_input / upload.invalid_suffix"
// @Failure      413   {object} map[string]string    "upload.file_too_large"
// @Failure      500   {object} map[string]string    "upload.internal"
// @Router       /admin/ajax/upload [post]
func (h *UploadHandler) Upload(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "upload.invalid_input",
			"message": "missing or unreadable form field 'file'",
		})
		return
	}

	topic := c.DefaultQuery("topic", "admin")

	f, err := fh.Open()
	if err != nil {
		// 文件已经在临时盘 / 内存里但 Open 失败 —— 内部错误。
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "upload.internal",
			"message": "failed to open uploaded file",
		})
		return
	}
	defer f.Close()

	res, err := h.svc.Upload(&upload.UploadInput{
		Topic:        topic,
		OriginalName: fh.Filename,
		Reader:       f,
	})
	if err != nil {
		switch {
		case errors.Is(err, upload.ErrInvalidInput):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "upload.invalid_input",
				"message": "invalid upload input",
			})
		case errors.Is(err, upload.ErrInvalidSuffix):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "upload.invalid_suffix",
				"message": "file suffix not allowed",
			})
		case errors.Is(err, upload.ErrFileTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"code":    "upload.file_too_large",
				"message": "file too large",
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "upload.internal",
				"message": "upload failed",
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"message": "ok",
		"data": uploadResponse{
			File: uploadFile{
				URL:        res.Url,
				StoredPath: res.StoredPath,
				Size:       res.Size,
				Suffix:     res.Suffix,
			},
		},
	})
}
