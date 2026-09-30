// Package admin 中 init.go —— 后台初始化服务 InitService。
//
// 调用方（InitHandler）在管理员登录后访问 `/admin/init` 时触发，
// 一次拉齐三件事：
//  1. 当前管理员信息（id / username / nickname / avatar / last_login_* / super）
//  2. 站点基础配置（name / record_number / version 三键，缺失填空串）
//  3. 当前管理员持有的菜单规则（admin_rule 行投影为 vue-router 兼容结构）
//
// 超级管理员走 RuleRepository.ListActiveAsMenu() 直接拿全量启用规则；
// 普通管理员走 PermissionManager.GetRules(uid)（已含 status=1 + 去重 + weigh 排序）。
// 两条路径汇合后统一过 routeFromRule 转换为前端 vue-router 友好的 map 结构。
package admin

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/infra/database"
	"ai-go-mall/internal/model"
	adminRepo "ai-go-mall/internal/repository/admin"
)

// InitAdmin 是 init 响应里的管理员字段。
//
// 比 login 响应的 adminInfo 多 super 字段（来自 Permission Manager.IsSuperAdmin），
// 其余字段与既有 adminInfo 风格保持一致。
type InitAdmin struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	Nickname    string `json:"nickname"`
	Avatar      string `json:"avatar"`
	LastLoginAt string `json:"last_login_at,omitempty"`
	LastLoginIp string `json:"last_login_ip,omitempty"`
	Super       bool   `json:"super"`
}

// InitResponse 是 `/admin/init` 端点的响应体。
//
// 字段命名遵循 init 服务自身的契约；JSON tag 决定 wire 形状。
type InitResponse struct {
	Admin      InitAdmin         `json:"admin"`
	SiteConfig map[string]string `json:"site_config"`
	Menus      []map[string]any  `json:"menus"`
}

// 站点配置固定查询的 name 集合 —— InitService 唯一接受的列表。
// 缺失行 → 空串，不返回整行（前端只关心 value）。
var initSiteConfigNames = []string{"name", "record_number", "version"}

// InitService 接口暴露给 handler 的唯一方法。
//
// 该接口只用于 Init 场景（后台启动初始化），不复用现有 Service 接口 ——
// Login / Logout 走 admin.Service，CRUD 走 CRUDService，Init 业务调 Permission Manager +
// Config Repository，与既有抽象层级不重叠。
type InitService interface {
	// Init 聚合当前管理员信息、站点配置、菜单规则，返回 InitResponse。
	//
	// 错误语义：
	//   - 当前管理员 nil（已被删除）：返回 ErrAdminDisabled（403）
	//   - admin.Status != 1：返回 ErrAccountDisabled（403）
	//   - 其他错误（DB / 转换器）：原样返回（handler 映射 500）
	Init(c *gin.Context, uid uint) (*InitResponse, error)
}

// initService 是 InitService 的默认实现。
//
// 依赖：
//   - repo：管理员主键查询（仅作 nil 兜底校验）
//   - pm：Permission Manager，提供 IsSuperAdmin + GetRules
//   - rules：规则 Repository，提供 ListActiveAsMenu（超管路径）
//   - cfg：config Repository，提供 ListByNames（站点配置）
//
// 注：admin 的状态校验在 service 层做，handler 不重复。
type initService struct {
	repo  adminRepo.AdminRepository
	pm    *Manager
	rules adminRepo.RuleRepository
	cfg   adminRepo.ConfigRepository
}

// NewInitService 构造一个 InitService。
//
// 四个依赖均为接口，可在测试中整体替换为 mock。
func NewInitService(repo adminRepo.AdminRepository, pm *Manager, rules adminRepo.RuleRepository, cfg adminRepo.ConfigRepository) InitService {
	return &initService{
		repo:  repo,
		pm:    pm,
		rules: rules,
		cfg:   cfg,
	}
}

// NewInitServiceDefault 用 database.Get() + 默认实现装配一个 InitService。
//
// 启动期（cmd/serve）一般只调用一次；持有返回的 InitService 复用即可。
func NewInitServiceDefault() InitService {
	db := database.Get()
	return NewInitService(
		adminRepo.NewAdminRepository(db),
		Default(),
		adminRepo.NewRuleRepository(db),
		adminRepo.NewConfigRepository(db),
	)
}

// Init 实现 InitService.Init。详细数据流见 design D5 / D9 / D11。
func (s *initService) Init(c *gin.Context, uid uint) (*InitResponse, error) {
	// 1) admin 入口校验：必须存在 + 启用。
	adm, err := s.repo.GetByID(uid)
	if err != nil {
		return nil, err
	}
	if adm == nil {
		return nil, ErrAccountDisabled
	}
	if adm.Status != 1 {
		return nil, ErrAccountDisabled
	}

	// 2) super 标记（Permission Manager 是唯一权威）。
	super := s.pm.IsSuperAdmin(uid)

	// 3) 站点配置查询：固定三个 name，缺失键填空串。
	siteConfig, err := s.loadSiteConfig(c.Request.Context())
	if err != nil {
		return nil, err
	}

	// 4) 菜单规则查询：超管走 ListActiveAsMenu，普通走 PermissionManager.GetRules。
	rules, err := s.loadMenus(c.Request.Context(), uid, super)
	if err != nil {
		return nil, err
	}

	// 5) 组装响应。
	return &InitResponse{
		Admin:      toInitAdmin(adm, super),
		SiteConfig: siteConfig,
		Menus:      rules,
	}, nil
}

// loadSiteConfig 查 config 表，返回固定三键 map，缺失键填空串。
func (s *initService) loadSiteConfig(ctx context.Context) (map[string]string, error) {
	rows, err := s.cfg.ListByNames(initSiteConfigNames)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(initSiteConfigNames))
	for _, name := range initSiteConfigNames {
		out[name] = "" // 显式初始化为 ""
	}
	for _, row := range rows {
		if row.Value != nil {
			out[row.Name] = *row.Value
		}
	}
	return out, nil
}

// loadMenus 拉 AdminRule 行并转换为 vue-router 兼容结构。
//
// 超管直接全量启用规则（一次 DB 读），普通走 Permission Manager（已排序）。
// 两路径都过 routeFromRule 转换，保证响应形状一致。
func (s *initService) loadMenus(ctx context.Context, uid uint, super bool) ([]map[string]any, error) {
	if super {
		rules, err := s.rules.ListActiveAsMenu()
		if err != nil {
			return nil, err
		}
		return rulesToRoutes(rules), nil
	}
	rules := s.pm.GetRules(uid)
	return rulesToRoutes(rules), nil
}

// rulesToRoutes 把 AdminRule 行集合统一过 routeFromRule 转 map 切片。
func rulesToRoutes(rules []model.AdminRule) []map[string]any {
	out := make([]map[string]any, 0, len(rules))
	for i := range rules {
		out = append(out, routeFromRule(&rules[i]))
	}
	return out
}

// toInitAdmin 把 *model.Admin + super 标志投影成 InitAdmin。
func toInitAdmin(adm *model.Admin, super bool) InitAdmin {
	info := InitAdmin{
		ID:          adm.ID,
		Username:    adm.Username,
		Nickname:    adm.Nickname,
		Avatar:      adm.Avatar,
		LastLoginIp: adm.LastLoginIp,
		Super:       super,
	}
	if adm.LastLoginAt != nil {
		info.LastLoginAt = adm.LastLoginAt.Format("2006-01-02T15:04:05Z07:00")
	}
	return info
}

// routeFromRule 把单条 AdminRule 行转换为 vue-router 兼容结构（map[string]any）。
//
// 纯函数，无 IO。便于单元测试覆盖 spec ADDED Requirement 6 的退化规则。
//
// 退化规则：
//   - name 为空 → "rule-{id}"
//   - path 为空 → "/rule-{id}"（与 name 退化对称）
//   - component 为空 → "404"（前端 router 走通用占位）
//   - OpenType 为 nil → meta.openType 字段不写入（区别于显式设为某个值）
//
// type == "node" 的规则仍转换（spec 要求出现在 menus 数组中），
// 但路由注册由前端按 meta.menuType == 'node' 跳过。
func routeFromRule(r *model.AdminRule) map[string]any {
	name := r.Name
	if name == "" {
		name = fmt.Sprintf("rule-%d", r.ID)
	}
	path := r.Path
	if path == "" {
		path = fmt.Sprintf("/rule-%d", r.ID)
	}
	component := r.Component
	if component == "" {
		component = "404"
	}

	meta := map[string]any{
		"title":     r.Title,
		"icon":      r.Icon,
		"keepalive": r.Keepalive,
		"menuType":  string(r.Type),
		"weigh":     r.Weigh,
		"id":        r.ID,
		"pid":       r.Pid,
		"extend":    string(r.Extend),
		"url":       r.Url,
	}
	if r.OpenType != nil {
		meta["openType"] = string(*r.OpenType)
	}

	return map[string]any{
		"path":      path,
		"name":      name,
		"component": component,
		"meta":      meta,
	}
}

// 编译期断言：*initService 必须实现 InitService。
var _ InitService = (*initService)(nil)
