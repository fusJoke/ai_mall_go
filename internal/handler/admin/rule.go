// Package admin —— 菜单规则实体的 HTTP 处理层（add-admin-rule-management）。
//
// 与管理员实体的 Handler 同模式：嵌入 *handler.BaseHandler[model.AdminRule]
// 自动获得 5 条通用 CRUD；在本结构上叠加 3 条业务专属方法（ToggleStatus /
// BatchDelete / ListAll）；Create / EditPost / Delete 三个方法被覆盖以映射
// 业务专属 sentinel 错误（PID 环 / PID 不存在 / 有子节点）。
package admin

import (
	"errors"
	"net/http"
	"reflect"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/handler"
	"ai-go-mall/internal/model"
	adminSvc "ai-go-mall/internal/service/admin"
)

// RuleHandler 是菜单规则实体的控制器。
//
// 通过嵌入 *handler.BaseHandler[model.AdminRule] 复用 5 条通用 CRUD 的处理函数；
// 额外持有 svc（adminSvc.RuleService）以便调用 ToggleStatus / BatchDelete /
// ListAll 三个业务专属方法 —— BaseHandler 字段被类型擦除为
// service.CRUDService[T]，访问不到扩展方法。
type RuleHandler struct {
	*handler.BaseHandler[model.AdminRule]
	svc adminSvc.RuleService
}

// NewRuleHandler 接收 rule service，返回 *RuleHandler。
//
// 注意：BaseHandler 已经持有同一个 svc（构造时强转 CRUDService），所以
// h.svc 与 h.BaseHandler.CRUDService 指向同一对象。
func NewRuleHandler(svc adminSvc.RuleService) *RuleHandler {
	return &RuleHandler{
		BaseHandler: handler.NewBaseHandler[model.AdminRule](svc),
		svc:         svc,
	}
}

// toggleStatusRequest 是 POST /admin/rule/toggle-status 的请求体。
type ruleToggleStatusRequest struct {
	ID uint `json:"id" binding:"required"`
}

// ToggleStatus 处理 POST /admin/rule/toggle-status。
//
// 错误映射：gorm.ErrRecordNotFound → 404；其他 → 500。
//
// 不联动 token 吊销（规则不绑会话，design D2）。
//
// @Summary  切换规则启用状态（启用 ↔ 禁用）
// @Tags     rule
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body admin.ruleToggleStatusRequest true "规则 ID"
// @Success  200 {object} map[string]string "rule.toggle_status.ok"
// @Failure  400 {object} map[string]string "rule.toggle_status.invalid_input"
// @Failure  404 {object} map[string]string "rule.toggle_status.not_found"
// @Failure  500 {object} map[string]string "rule.toggle_status.internal"
// @Router   /admin/rule/toggle-status [post]
func (h *RuleHandler) ToggleStatus(c *gin.Context) {
	var req ruleToggleStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "rule.toggle_status.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.ToggleStatus(c, req.ID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"code":    "rule.toggle_status.not_found",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "rule.toggle_status.internal",
			"message": "toggle status failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": "rule.toggle_status.ok", "message": "ok"})
}

// batchDeleteRequest 是 POST /admin/rule/batch-delete 的请求体。
type ruleBatchDeleteRequest struct {
	IDs []uint `json:"ids"`
}

// BatchDelete 处理 POST /admin/rule/batch-delete。
//
// 空 ids / 全是「有子 / 不存在」→ 仍返 200 + {deleted:0, skipped:n}（service 已处理）。
//
// @Summary  批量删除规则（自动跳过有子节点与不存在的 id）
// @Tags     rule
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body admin.ruleBatchDeleteRequest true "规则 ID 列表"
// @Success  200 {object} map[string]int "rule.batch_delete.ok"
// @Failure  400 {object} map[string]string "rule.batch_delete.invalid_input"
// @Failure  500 {object} map[string]string "rule.batch_delete.internal"
// @Router   /admin/rule/batch-delete [post]
func (h *RuleHandler) BatchDelete(c *gin.Context) {
	var req ruleBatchDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "rule.batch_delete.invalid_input",
			"message": err.Error(),
		})
		return
	}
	deleted, skipped, err := h.svc.BatchDelete(c, req.IDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "rule.batch_delete.internal",
			"message": "batch delete failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"deleted": deleted,
		"skipped": skipped,
	})
}

// ListAll 处理 GET /admin/rule/all。
//
// 返回全量启用规则按 weigh ASC, id ASC 排序，用于管理页「父节点」下拉框。
// 注意：响应体直接是 AdminRule 数组（不带分页包装）。
//
// @Summary  列出全部启用规则（无分页）
// @Tags     rule
// @Produce  json
// @Security BearerAuth
// @Success  200 {array} model.AdminRule "rule.list_all.ok"
// @Failure  500 {object} map[string]string "rule.list_all.internal"
// @Router   /admin/rule/all [get]
func (h *RuleHandler) ListAll(c *gin.Context) {
	items, err := h.svc.ListAll(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "rule.list_all.internal",
			"message": "list all failed",
		})
		return
	}
	c.JSON(http.StatusOK, items)
}

// --- BaseHandler 默认方法覆盖：业务专属错误码映射 ---
//
// Create / EditPost / Delete 三个方法在 BaseHandler 默认实现里把所有错误一律
// 500 出去 —— 但 service 层已经把业务约束（PID 环 / PID 不存在 / 有子节点）转成
// sentinel 错误，handler 在这里分门别类把它们映射成 400 + 具体错误码字符串，
// 让前端可以做精细化提示。

// Create 覆盖 BaseHandler.Create：处理 PID 业务错误。
//
// service.Create 已经做 PID 校验（design D3）：PID 自指 / PID 是子孙 /
// PID 不存在 → 业务 sentinel 错误；DB 异常 → 其他错误。
//
// @Summary  新建菜单规则
// @Tags     rule
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body model.AdminRule true "规则字段（pid=0 表示顶级）"
// @Success  200 {object} model.AdminRule "rule.create.ok"
// @Failure  400 {object} map[string]string "rule.create.invalid_input / rule.create.pid_cycle / rule.create.pid_not_found"
// @Failure  500 {object} map[string]string "rule.create.internal"
// @Router   /admin/rule/create [post]
func (h *RuleHandler) Create(c *gin.Context) {
	var entity model.AdminRule
	if err := c.ShouldBindJSON(&entity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "rule.create.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.Create(c, &entity); err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrPIDCycle):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "rule.create.pid_cycle",
				"message": err.Error(),
			})
		case errors.Is(err, adminSvc.ErrPIDNotFound):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "rule.create.pid_not_found",
				"message": err.Error(),
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "rule.create.internal",
				"message": "create failed",
			})
		}
		return
	}
	c.JSON(http.StatusOK, entity)
}

// EditPost 覆盖 BaseHandler.EditPost：处理 PID 业务错误。
//
// service.Update 已经做 PID 环校验（design D3）。
//
// @Summary  提交修改菜单规则
// @Tags     rule
// @Accept   json
// @Produce  json
// @Security BearerAuth
// @Param    req body model.AdminRule true "规则字段（必须含 id）"
// @Success  200 {object} model.AdminRule "rule.edit.ok"
// @Failure  400 {object} map[string]string "rule.edit.invalid_input / rule.edit.pid_cycle / rule.edit.pid_not_found"
// @Failure  500 {object} map[string]string "rule.edit.internal"
// @Router   /admin/rule/edit [post]
func (h *RuleHandler) EditPost(c *gin.Context) {
	var entity model.AdminRule
	if err := c.ShouldBindJSON(&entity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "rule.edit.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := requireNonZeroIDByReflect(&entity); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "rule.edit.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.Update(c, &entity); err != nil {
		switch {
		case errors.Is(err, adminSvc.ErrPIDCycle):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "rule.edit.pid_cycle",
				"message": err.Error(),
			})
		case errors.Is(err, adminSvc.ErrPIDNotFound):
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "rule.edit.pid_not_found",
				"message": err.Error(),
			})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    "rule.edit.internal",
				"message": "edit failed",
			})
		}
		return
	}
	c.JSON(http.StatusOK, entity)
}

// Delete 覆盖 BaseHandler.Delete：处理「有子节点」业务错误。
//
// service.Delete 已经做 hasChildren 检查（design D3）。
//
// @Summary  删除菜单规则（有子节点时拒绝）
// @Tags     rule
// @Security BearerAuth
// @Param    id query int true "规则 ID"
// @Success  200 {object} map[string]any "rule.delete.ok"
// @Failure  400 {object} map[string]string "rule.delete.invalid_input / rule.delete.has_children"
// @Failure  500 {object} map[string]string "rule.delete.internal"
// @Router   /admin/rule/delete [post]
func (h *RuleHandler) Delete(c *gin.Context) {
	id, err := parseRuleID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    "rule.delete.invalid_input",
			"message": err.Error(),
		})
		return
	}
	if err := h.svc.Delete(c, id); err != nil {
		if errors.Is(err, adminSvc.ErrHasChildren) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "rule.delete.has_children",
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    "rule.delete.internal",
			"message": "delete failed",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// parseRuleID 仅从 query (?id=) 取；与 router 注册保持一致（design D5）。
//
// 与 admin.go 的 parseID 同实现；放本文件避免跨包升级成 export helper。
func parseRuleID(c *gin.Context) (int64, error) {
	raw := c.Query("id")
	if raw == "" {
		return 0, errors.New("missing id")
	}
	id, err := parseInt64Strict(raw)
	if err != nil {
		return 0, errors.New("invalid id")
	}
	return id, nil
}

// parseInt64Strict 严格十进制 64 位解析；非数字字符 / 溢出 → 报错。
//
// 比 strconv.ParseInt 严：拒绝负号、空串、科学计数；本项目 ID 全部为正整数，
// 拒绝负号能挡掉少量脏输入。
func parseInt64Strict(s string) (int64, error) {
	if s == "" {
		return 0, errors.New("empty")
	}
	var n int64
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if ch < '0' || ch > '9' {
			return 0, errors.New("not a digit")
		}
		n = n*10 + int64(ch-'0')
		if n < 0 { // 溢出
			return 0, errors.New("overflow")
		}
	}
	return n, nil
}

// requireNonZeroIDByReflect 通过反射校验实体的 ID 字段非零。
//
// 与 admin.go 的 requireNonZeroID 行为一致；这里独立一份避免在 admin 包内
// 升级 export helper —— admin 包只需要这一种解析（ID uint），与 admin.go
// 内的版本语义一致。
func requireNonZeroIDByReflect(entity any) error {
	v := reflect.ValueOf(entity)
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			return errors.New("edit: nil entity")
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return errors.New("edit: entity must be a struct")
	}
	idField := v.FieldByName("ID")
	if !idField.IsValid() {
		return errors.New("edit: entity has no ID field")
	}
	switch idField.Kind() {
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if idField.Uint() == 0 {
			return errors.New("edit: missing or zero id")
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		if idField.Int() == 0 {
			return errors.New("edit: missing or zero id")
		}
	default:
		return errors.New("edit: ID field must be an integer")
	}
	return nil
}
