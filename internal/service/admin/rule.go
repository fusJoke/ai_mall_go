// Package admin 菜单规则 Service 层（add-admin-rule-management 起新增）。
//
// 通用 CRUD 走 service.CRUDService[model.AdminRule] 转发（通过
// ruleCRUDAdapter 把 AdminRule.ID 的 uint 与 CRUDRepository[int64] 适配）；
// 业务专属方法（ToggleStatus / BatchDelete / ListAll / ValidatePID）由本包
// 实现，叠加 PID 环校验与"有子节点拒绝删"等业务约束。
package admin

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	adminRepo "ai-go-mall/internal/repository/admin"
	"ai-go-mall/internal/service"
)

// 菜单规则业务专属 sentinel 错误（design D7）。
//
// handler 把这些错误映射到具体 HTTP 状态码 + 错误码字符串；
// service 层不掺 HTTP 概念。
var (
	// ErrPIDCycle 表示「把 rule.X 的 pid 设为 rule.X 自身或 rule.X 的子孙」。
	// 出现该错误时调用方应拒绝 update / create 并提示"不能将规则设为自身或子孙的子节点"。
	ErrPIDCycle = errors.New("rule: pid forms cycle")

	// ErrHasChildren 表示「rule.X 仍有未软删的子规则，不能删」。
	// 调用方应先删子规则或迁移子规则到祖父，再删父规则。
	ErrHasChildren = errors.New("rule: rule has children")

	// ErrPIDNotFound 表示「newPID 引用的父规则不存在或已软删」。
	// 与 PID 环区分：newPID=合法但 rule.X 的 id 是 0（create 路径首次新增）允许，
	// newPID=合法但 rule.X 不是 newPID 的祖先（自上溯验证通过）允许；该错误
	// 仅在「newPID 本身就不存在」时返回。
	ErrPIDNotFound = errors.New("rule: pid not found")
)

// RuleService 是菜单规则实体的业务接口。
//
// handler 通过此接口调用；具体实现由 NewRuleService 返回。
type RuleService interface {
	service.CRUDService[model.AdminRule]

	// ValidatePID 检查"把 rule.id 的 pid 设为 newPID"是否会形成 PID 环，
	// 或 newPID 是否指向已软删 / 不存在的父规则。
	//
	// 三种返回：
	//   - newPID == 0：永远合法（顶级），返回 nil；
	//   - newPID == id：自父，返回 ErrPIDCycle；
	//   - BFS 上溯 newPID 的祖先链：命中 id 则 newPID 是 id 的子孙，返回 ErrPIDCycle；
	//   - newPID 在第一跳不存在 / 已软删：返回 ErrPIDNotFound；
	//   - 链上其他位置 not-found：视为「祖先链断在合法点」，返回 nil。
	//
	// visited map 防御"现存数据已有环"导致的死循环；链深度上限由 BFS 自带
	// 检测 + visited 守护，最坏 O(visited set size) 次 SELECT。
	ValidatePID(c *gin.Context, id, newPID uint) error

	// ToggleStatus 切换 Status：1↔0。不联动 token 吊销（规则不绑会话，design D2）。
	// 不存在 → gorm.ErrRecordNotFound。
	ToggleStatus(c *gin.Context, id uint) error

	// BatchDelete 软删一组 id；删除前检查每个 id 是否有子规则，有则跳过（不删除），
	// 计入 skipped；不存在的 id / 已软删的 id 也计入 skipped。
	// 返回 (deleted, skipped, err)。
	BatchDelete(c *gin.Context, ids []uint) (deleted int, skipped int, err error)

	// ListAll 返回全量启用规则（status=1，未软删），按 weigh ASC, id ASC 排序。
	// 复用 RuleRepository.ListActiveAsMenu；用于菜单树父节点下拉框。
	ListAll(c *gin.Context) ([]model.AdminRule, error)
}

// baseRuleService 是 RuleService 的默认实现。
//
// 嵌入 service.CRUDService[model.AdminRule] 转发通用 CRUD；
// 额外持有 repo（adminRepo.RuleRepository）以拿到 PID 校验与 ToggleStatus
// 所需的专用方法。
type baseRuleService struct {
	service.CRUDService[model.AdminRule]
	repo adminRepo.RuleRepository
}

// NewRuleService 接收 rule repo，返回 RuleService 接口。
func NewRuleService(repo adminRepo.RuleRepository) RuleService {
	return &baseRuleService{
		CRUDService: service.NewBaseCRUDService[model.AdminRule](adminRepo.NewRuleCRUDAdapter(repo)),
		repo:        repo,
	}
}

// ValidatePID 实现 RuleService.ValidatePID。详细策略见接口注释。
func (s *baseRuleService) ValidatePID(c *gin.Context, id, newPID uint) error {
	if newPID == 0 {
		return nil // 顶级永远合法
	}
	if id != 0 && newPID == id {
		return ErrPIDCycle // 自父
	}

	// newPID 的第一跳：必须存在（且未软删）。
	firstPID, err := s.repo.GetPID(c, newPID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrPIDNotFound
		}
		return err
	}

	// 沿 newPID 的 pid 链上溯，命中 id 即"newPID 是 id 的子孙"。
	cur := firstPID
	visited := make(map[uint]struct{})
	for cur != 0 {
		if id != 0 && cur == id {
			return ErrPIDCycle
		}
		if _, seen := visited[cur]; seen {
			break // 防御性：现存数据已有环，终止以免无限循环
		}
		visited[cur] = struct{}{}
		next, err := s.repo.GetPID(c, cur)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break // 链上某祖先不存在：视为合法终止点
			}
			return err
		}
		cur = next
	}
	return nil
}

// Create 覆盖 BaseCRUDService.Create：转发前先做 PID 校验（design D3）。
//
// service.CRUDService.Create 已经做 nil 检查；本方法只关注 PID 业务约束。
func (s *baseRuleService) Create(c *gin.Context, entity *model.AdminRule) error {
	if entity == nil {
		return errors.New("service: nil entity")
	}
	if err := s.ValidatePID(c, 0, entity.Pid); err != nil {
		return err
	}
	return s.CRUDService.Create(c, entity)
}

// Update 覆盖 BaseCRUDService.Update：转发前先做 PID 环校验（design D3）。
//
// service 必须先确认行存在（CRUDRepository.Update 是 GORM Save，缺主键时 INSERT）；
// 校验失败时 entity 已修改但还没落库，调用方回退到原 entity。
func (s *baseRuleService) Update(c *gin.Context, entity *model.AdminRule) error {
	if entity == nil {
		return errors.New("service: nil entity")
	}
	if err := s.ValidatePID(c, entity.ID, entity.Pid); err != nil {
		return err
	}
	return s.CRUDService.Update(c, entity)
}

// Delete 覆盖 BaseCRUDService.Delete：转发前检查是否有子节点（design D3）。
//
// 有子 → ErrHasChildren；无子 → 转发删除。
func (s *baseRuleService) Delete(c *gin.Context, id int64) error {
	has, err := s.repo.HasChildren(c, uint(id))
	if err != nil {
		return err
	}
	if has {
		return ErrHasChildren
	}
	return s.CRUDService.Delete(c, id)
}

// ToggleStatus 实现 RuleService.ToggleStatus（design D2）。
//
// 1→0 / 0→1 二选一；不联动 token（规则不绑会话）。
// 不存在 → gorm.ErrRecordNotFound 透传。
func (s *baseRuleService) ToggleStatus(c *gin.Context, id uint) error {
	current, err := s.repo.GetByID(c, id)
	if err != nil {
		return err
	}
	if current == nil {
		return gorm.ErrRecordNotFound
	}

	var newStatus int8
	if current.Status == 1 {
		newStatus = 0
	} else {
		newStatus = 1
	}
	return s.repo.UpdateStatus(c, id, newStatus)
}

// BatchDelete 实现 RuleService.BatchDelete（design D2）。
//
// 遍历 ids：对每个 id 调 HasChildren + GetByID 判存在性 + 调 Delete；
// 不存在 / 有子 / 已软删 三种情况都计入 skipped；其余计入 deleted。
//
// 单次顺序循环不做并发（admin 管理页 UI 串行调用即可）；ids 上限由前端 /
// 上层控制，本层不做截断。
func (s *baseRuleService) BatchDelete(c *gin.Context, ids []uint) (int, int, error) {
	if len(ids) == 0 {
		return 0, 0, nil
	}

	deleted := 0
	skipped := 0
	for _, id := range ids {
		// 存在性检查：GetByID 返回 gorm.ErrRecordNotFound 或 nil → 跳过。
		current, err := s.repo.GetByID(c, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				skipped++
				continue
			}
			return deleted, skipped, err
		}
		if current == nil {
			skipped++
			continue
		}
		// 有子检查。
		has, err := s.repo.HasChildren(c, id)
		if err != nil {
			return deleted, skipped, err
		}
		if has {
			skipped++
			continue
		}
		// 删除。
		if err := s.CRUDService.Delete(c, int64(id)); err != nil {
			return deleted, skipped, err
		}
		deleted++
	}
	return deleted, skipped, nil
}

// ListAll 实现 RuleService.ListAll（design D2 / D8）。
//
// 复用 RuleRepository.ListActiveAsMenu；走单条 SQL 拉全量启用规则。
func (s *baseRuleService) ListAll(c *gin.Context) ([]model.AdminRule, error) {
	return s.repo.ListActiveAsMenu()
}

// 编译期断言：baseRuleService 必须实现 RuleService。
var _ RuleService = (*baseRuleService)(nil)
