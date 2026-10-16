package admin

import (
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/model"
	"ai-go-mall/internal/repository"
)

// RuleRepository 定义规则域的查询 / 写入接口（add-admin-rule-management 起
// 扩展为含写方法的单接口实体仓储，与 group_repository / config_repository
// 风格对齐 —— design D4）。
//
// 设计原则（与 design D5 / D9 / D10 / D4 一致）：
//   - 批量查询（ListByIDs / ListActiveIDs / ExistsByIDsAndName）走单条
//     `WHERE id IN (?)` SQL，避免 N+1（design D10）；
//   - ListActiveIDs 把 status=1 过滤推到 SQL 层，用于"超管全规则"路径，
//     不在 Manager 层再二次过滤（性能 + 简洁性 trade-off，design D9 例外）；
//   - ExistsByIDsAndName 同样把 status=1 推到 SQL，避免 Manager 拉行后
//     Go 端二次过滤；返回 bool 而非行集，节省内存；
//   - 写方法（Create / Update / Delete / DeleteBatch / UpdateStatus /
//     HasChildren）走 GORM 标准语义（与 admin_repository.go 既有 UpdateStatus
//     / DeleteBatch 风格一致），handler / service 不掺 GORM 调用。
//
// 注意：Create / Update / Delete 不嵌入 repository.CRUDRepository[model.AdminRule]
// —— AdminRule.ID 是 uint 而 CRUDRepository.Delete / GetByID 用 int64，
// 类型不兼容；service 层通过 ruleCRUDAdapter（见文件末尾）做转换后再走
// service.NewBaseCRUDService[model.AdminRule]。
type RuleRepository interface {
	// ListByIDs 按 id 集合批量取规则行（不过滤 status / deleted_at
	// 之外的任何业务字段；deleted_at 由 GORM 自动过滤）。
	//
	// 空 ids 返回空切片（不查 DB）。
	ListByIDs(ids []uint) ([]model.AdminRule, error)

	// ExistsByIDsAndName 判断"id 集合中存在一条 status=1 且 name=name 的规则"。
	// 用于 Manager.Check 的最后一步。
	ExistsByIDsAndName(ids []uint, name string) (bool, error)

	// ListActiveIDs 拉所有启用规则的 id 升序切片。用于超管路径：
	// Manager 检测到通配符 '*' 后用此接口取全规则 id 集合。
	ListActiveIDs() ([]uint, error)

	// ListActiveAsMenu 拉所有启用的 AdminRule 行，按 weigh ASC, id ASC 排序。
	// 用于后台 init 端点「超管路径」：一次拉齐菜单规则全量数据，避免
	// 先 ListActiveIDs 再 ListByIDs 两次往返。
	ListActiveAsMenu() ([]model.AdminRule, error)

	// Create 单行插入；GORM 自动维护 CreatedAt / UpdatedAt。
	// service.Create 在转发之前会先做 PID 校验（design D3）。
	Create(c *gin.Context, rule *model.AdminRule) error

	// Update 全量更新（GORM Save 语义），service.Update 在转发之前
	// 会先做 PID 环校验（design D3）。
	Update(c *gin.Context, rule *model.AdminRule) error

	// Delete 软删单条（design D3 中 Delete 路径 + service 层有子节点检查）。
	Delete(c *gin.Context, id uint) error

	// DeleteBatch 软删一组 id；空 ids 走短路返回 nil。
	DeleteBatch(c *gin.Context, ids []uint) error

	// UpdateStatus 仅更新 Status 一列（ToggleStatus 路径专用，省一次 SELECT）。
	UpdateStatus(c *gin.Context, id uint, status int8) error

	// GetPID 取单行 pid 列；service.validatePID 用此沿 pid 链上溯
	// 判环；主键索引每次 O(1)，最坏 O(depth)（design D3）。
	GetPID(c *gin.Context, id uint) (uint, error)

	// HasChildren 判断"id 下仍有未软删的子规则"，service.Delete / BatchDelete
	// 在删除前调此方法决定是否拒绝。命中即 true（不做计数，只需一条存在性）。
	HasChildren(c *gin.Context, id uint) (bool, error)

	// GetByID 按主键查一行；service.GetByID 转发到此。
	GetByID(c *gin.Context, id uint) (*model.AdminRule, error)

	// List 按 opts 分页查询（Page 从 1 计）；service.List 转发到此。
	List(c *gin.Context, opts repository.ListOptions) (items []model.AdminRule, total int64, err error)
}

// gormRuleRepository 是基于 *gorm.DB 的 RuleRepository 实现。
type gormRuleRepository struct {
	db *gorm.DB
}

// NewRuleRepository 构造一个 RuleRepository。
func NewRuleRepository(db *gorm.DB) RuleRepository {
	return &gormRuleRepository{db: db}
}

// ListByIDs 实现 RuleRepository.ListByIDs。
func (r *gormRuleRepository) ListByIDs(ids []uint) ([]model.AdminRule, error) {
	rules := make([]model.AdminRule, 0)
	if len(ids) == 0 {
		return rules, nil
	}
	err := r.db.Where("id IN ?", ids).Find(&rules).Error
	return rules, err
}

// ExistsByIDsAndName 实现 RuleRepository.ExistsByIDsAndName。
//
// 把 status=1 过滤推到 SQL（Count 走索引更高效）；空 ids 短路返回 false。
func (r *gormRuleRepository) ExistsByIDsAndName(ids []uint, name string) (bool, error) {
	if len(ids) == 0 {
		return false, nil
	}
	var count int64
	err := r.db.Model(&model.AdminRule{}).
		Where("id IN ? AND name = ? AND status = ?", ids, name, 1).
		Count(&count).Error
	return count > 0, err
}

// ListActiveIDs 实现 RuleRepository.ListActiveIDs。
func (r *gormRuleRepository) ListActiveIDs() ([]uint, error) {
	var ids []uint
	err := r.db.Model(&model.AdminRule{}).
		Where("status = ?", 1).
		Order("id ASC").
		Pluck("id", &ids).Error
	return ids, err
}

// ListActiveAsMenu 实现 RuleRepository.ListActiveAsMenu。
//
// 单条 `SELECT * FROM admin_rule WHERE status = 1 ORDER BY weigh ASC, id ASC`，
// GORM 自动过滤软删（deleted_at IS NULL）。返回切片为新分配的，调用方可自由修改。
func (r *gormRuleRepository) ListActiveAsMenu() ([]model.AdminRule, error) {
	rules := make([]model.AdminRule, 0)
	err := r.db.Where("status = ?", 1).
		Order("weigh ASC, id ASC").
		Find(&rules).Error
	return rules, err
}

// Create 实现 RuleRepository.Create。
//
// 走 request-scoped DB（design D4）；entity 为 nil 时按编程错误处理，
// 与 BaseRepository.Create 风格一致。GORM 自动维护 CreatedAt / UpdatedAt。
func (r *gormRuleRepository) Create(c *gin.Context, rule *model.AdminRule) error {
	if rule == nil {
		return errors.New("repository: nil rule")
	}
	return repository.DB(c).Create(rule).Error
}

// Update 实现 RuleRepository.Update。
//
// GORM Save 语义：按主键全列覆盖（含零值）。调用方（service.Update）在
// 转发之前会先做 PID 环校验（design D3）。
func (r *gormRuleRepository) Update(c *gin.Context, rule *model.AdminRule) error {
	if rule == nil {
		return errors.New("repository: nil rule")
	}
	return repository.DB(c).Save(rule).Error
}

// Delete 实现 RuleRepository.Delete。
//
// 软删：admin_rule 表含 gorm.DeletedAt，GORM 自动写 deleted_at。
// 不存在的 id GORM 不会报错（UPDATE 影响 0 行），幂等。
func (r *gormRuleRepository) Delete(c *gin.Context, id uint) error {
	return repository.DB(c).
		Where("id = ?", id).
		Delete(&model.AdminRule{}).Error
}

// DeleteBatch 实现 RuleRepository.DeleteBatch。
//
// 空 ids 短路返回 nil（不查 DB）；非空走单条 `WHERE id IN (?)` 软删，
// GORM 自动加 deleted_at IS NULL 过滤，不存在的行 / 已软删的行不报错。
func (r *gormRuleRepository) DeleteBatch(c *gin.Context, ids []uint) error {
	if len(ids) == 0 {
		return nil
	}
	return repository.DB(c).
		Where("id IN ?", ids).
		Delete(&model.AdminRule{}).Error
}

// UpdateStatus 实现 RuleRepository.UpdateStatus。
//
// 走 Updates(map[string]any{...})：map 形式的 Updates 只更新指定列，
// 不会覆盖其他业务字段；省一次 SELECT + 锁。
func (r *gormRuleRepository) UpdateStatus(c *gin.Context, id uint, status int8) error {
	return repository.DB(c).
		Model(&model.AdminRule{}).
		Where("id = ?", id).
		Updates(map[string]any{"status": status}).Error
}

// GetPID 实现 RuleRepository.GetPID。
//
// 必须用 Take 而不是 Pluck：**GORM 的 Pluck 在「查不到行」时不返回
// gorm.ErrRecordNotFound**（目标留在零值、err 仍为 nil），调用方就没有办法把
// 「父规则不存在」与「父规则 pid=0（顶级）」区分开 —— 前者必须报 ErrPIDNotFound，
// 后者是合法的上溯终止点。用 Pluck 会让 service.ValidatePID 的 not-found 分支
// 永远不可达，于是 pid 指向不存在节点的孤儿规则可以写进库。
// Take 在无行时返回 gorm.ErrRecordNotFound，语义正确。
//
// 只 Select pid 一列，主键索引 O(1)；service.validatePID 沿此列上溯判环，
// 最坏 O(depth)。
func (r *gormRuleRepository) GetPID(c *gin.Context, id uint) (uint, error) {
	var rule model.AdminRule
	err := repository.DB(c).
		Select("pid").
		Where("id = ?", id).
		Take(&rule).Error
	if err != nil {
		return 0, err
	}
	return rule.Pid, nil
}

// HasChildren 实现 RuleRepository.HasChildren。
//
// 单条 `SELECT id FROM admin_rule WHERE pid = ? AND deleted_at IS NULL LIMIT 1`：
// 命中任意子节点即 true，不做计数 —— service 层只关心"有没有"而不是"有几个"。
// 空 pid=0 也走这条 SQL（pid=0 是顶级标志，理论上不该有子，但 GORM 仍会查）；
// 当前设计 pid=0 是顶级，调用方（service.Delete）只在 pid!=0 的节点上调用。
//
// 判据是「有没有查到行」而不是「字段值是否非零」：这里同样不用 Pluck ——
// 即便 Pluck 在这条上恰好也能work（子节点 id 必非 0），同文件里两种写法并存
// 迟早会被复制错地方。用 Take + errors.Is(ErrRecordNotFound) 表意更明确。
func (r *gormRuleRepository) HasChildren(c *gin.Context, id uint) (bool, error) {
	var child model.AdminRule
	err := repository.DB(c).
		Select("id").
		Where("pid = ?", id).
		Take(&child).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetByID 实现 RuleRepository.GetByID。
//
// 主键查单行；记录不存在时返回 gorm.ErrRecordNotFound。
func (r *gormRuleRepository) GetByID(c *gin.Context, id uint) (*model.AdminRule, error) {
	var rule model.AdminRule
	err := repository.DB(c).First(&rule, id).Error
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ruleFilterColumns 是 admin_rule 列表允许模糊过滤的列（= ListOptions.Filters 的 key）。
//
// 与 handler.listFilterKeys 的白名单对应；这里再固定一次列名，保证拼进 SQL 的是
// 常量而不是调用方传进来的字符串。
const (
	ruleFilterTitle = "title"
	ruleFilterName  = "name"
)

// escapeLikePattern 转义 LIKE 的通配符，让关键词按**字面量**匹配。
//
// 不转义的话，用户输入 `%` 会被当成通配符而匹配到全部行（`_` 同理），
// 与 spec 的「fuzzy-match」语义不符。MySQL 默认转义符是反斜杠，
// 因此无需额外写 ESCAPE 子句。
func escapeLikePattern(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// filteredQuery 按 opts.Filters 构造列表查询（每次返回**新的** *gorm.DB）。
//
// 必须每次重建而不是复用同一个 builder：`Count` 会往 Statement 里塞
// `SELECT count(*)`，复用会让随后的 Find 也带上计数子句。
//
// 不认识的 key 一律忽略（见 repository.ListOptions.Filters 的约定）——
// 这样给别的实体加过滤时，rule 列表不会因为多收一个参数而报错。
func (r *gormRuleRepository) filteredQuery(c *gin.Context, filters map[string]string) *gorm.DB {
	db := repository.DB(c).Model(&model.AdminRule{})
	if v := strings.TrimSpace(filters[ruleFilterTitle]); v != "" {
		db = db.Where("title LIKE ?", "%"+escapeLikePattern(v)+"%")
	}
	if v := strings.TrimSpace(filters[ruleFilterName]); v != "" {
		db = db.Where("name LIKE ?", "%"+escapeLikePattern(v)+"%")
	}
	return db
}

// List 实现 RuleRepository.List。
//
// 走 repository.DB(c) 拿 request-scoped DB，Page 从 1 计。
// total 通过单独 Count 取，避免 offset 越大越慢；Count 与 Find 共用
// filteredQuery 的条件，保证「总数」与「当前页」出自同一筛选口径。
//
// 模糊匹配依赖列的排序规则（utf8mb4_general_ci）实现大小写不敏感，
// 与 spec scenario「Filtered by title」的 case-insensitive 要求一致。
func (r *gormRuleRepository) List(c *gin.Context, opts repository.ListOptions) ([]model.AdminRule, int64, error) {
	var total int64
	if err := r.filteredQuery(c, opts.Filters).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	items := make([]model.AdminRule, 0)
	if total == 0 {
		return items, total, nil
	}

	offset := (opts.Page - 1) * opts.PageSize
	if err := r.filteredQuery(c, opts.Filters).
		Offset(offset).
		Limit(opts.PageSize).
		Find(&items).Error; err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ruleCRUDAdapter 把 RuleRepository（uint id）适配成 repository.CRUDRepository[model.AdminRule]
// （int64 id），供 service.NewBaseCRUDService[model.AdminRule] 使用。
//
// 只暴露 service.CRUDService 需要的方法（Create / List / GetByID / Update / Delete）；
// int64 ↔ uint 转换是项目内 AdminRule.ID 与 CRUDRepository 标准签名之间的固定差异，
// 放在 adapter 而非 service 层可让 service 不感知 ID 类型。
type ruleCRUDAdapter struct {
	rr RuleRepository
}

// NewRuleCRUDAdapter 构造一个 repository.CRUDRepository[model.AdminRule]，
// 供 service.NewBaseCRUDService[model.AdminRule] 使用。
//
// service 层不应该直接感知 AdminRule.ID 是 uint 而 CRUDRepository 用 int64
// —— 这个适配器把转换隔离开。
func NewRuleCRUDAdapter(rr RuleRepository) *ruleCRUDAdapter {
	return &ruleCRUDAdapter{rr: rr}
}

func (a *ruleCRUDAdapter) Create(c *gin.Context, entity *model.AdminRule) error {
	if entity == nil {
		return errors.New("repository: nil entity")
	}
	return a.rr.Create(c, entity)
}

func (a *ruleCRUDAdapter) Update(c *gin.Context, entity *model.AdminRule) error {
	if entity == nil {
		return errors.New("repository: nil entity")
	}
	return a.rr.Update(c, entity)
}

func (a *ruleCRUDAdapter) Delete(c *gin.Context, id int64) error {
	return a.rr.Delete(c, uint(id))
}

func (a *ruleCRUDAdapter) GetByID(c *gin.Context, id int64) (*model.AdminRule, error) {
	return a.rr.GetByID(c, uint(id))
}

func (a *ruleCRUDAdapter) List(c *gin.Context, opts repository.ListOptions) ([]model.AdminRule, int64, error) {
	return a.rr.List(c, opts)
}

// 编译期断言：ruleCRUDAdapter 满足 repository.CRUDRepository[model.AdminRule]。
var _ repository.CRUDRepository[model.AdminRule] = (*ruleCRUDAdapter)(nil)
