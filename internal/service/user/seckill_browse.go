// Package user — seckill_browse.go 实现 C 端秒杀活动的浏览查询（List / Detail）。
//
// 与 draw.go 的 SeckillService（秒杀抽卡事务）分离：浏览是只读聚合，
// 不触碰 Redis 限购 / 库存预扣；两者共享 SeckillRepository。
//
// 剩余名额口径（D15）：
//   - 主来源：Redis GET seckill:stock:{id}（秒杀事务 DECR 后的实时值）；
//   - 兜底：Redis miss / Redis 不可用时走 DB —— total_stock − mall_stock_deduction_log
//     已扣数（扣减日志与订单同事务写入，DB 侧口径一致）；
//   - 两边都拿不到 → -1（前端显示为"未知"，不阻断浏览）。
//
// 可见性过滤（与盲盒列表同策略，design D9）：
//   - 活动 status=active 且 NOW() ∈ [start_at, end_at)（repo ListActive）；
//   - 关联盲盒 status=active 且 on_sale=true；
//   - 供应商未禁用。
//     抽卡路径（DrawSeckill）会再做一次防御性校验，浏览过滤只是展示层。
package user

import (
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/infra/cache"
	mallModel "ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// remainingUnknown 表示剩余名额暂不可知（Redis 与 DB 兜底都失败）。
const remainingUnknown = -1

// SeckillListItem 是 C 端秒杀活动列表 / 详情共用的展示投影。
//
// 白名单字段：不透出 supplier 商业字段（balance 等），盲盒只给展示所需。
type SeckillListItem struct {
	// ID 秒杀活动 ID（秒杀下单 POST /user/seckill/:id/draw 的 :id）。
	ID int64 `json:"id"`

	// BlindBoxID 关联盲盒 ID（跳转盲盒详情页用）。
	BlindBoxID int64 `json:"blind_box_id"`

	// BlindBoxName 盲盒名称。
	BlindBoxName string `json:"blind_box_name"`

	// BlindBoxCover 盲盒封面。
	BlindBoxCover string `json:"blind_box_cover"`

	// SupplierID 供应商 ID。
	SupplierID int64 `json:"supplier_id"`

	// SupplierName 供应商名称。
	SupplierName string `json:"supplier_name"`

	// OriginalPrice 盲盒原价。
	OriginalPrice float64 `json:"original_price"`

	// SeckillPrice 秒杀价。
	SeckillPrice float64 `json:"seckill_price"`

	// TotalStock 秒杀总名额。
	TotalStock int `json:"total_stock"`

	// RemainingStock 实时剩余名额（Redis 优先，DB 兜底；-1 = 未知）。
	RemainingStock int `json:"remaining_stock"`

	// PerUserLimit 每用户限购数量。
	PerUserLimit int `json:"per_user_limit"`

	// StartAt 活动开始时间。
	StartAt time.Time `json:"start_at"`

	// EndAt 活动结束时间。
	EndAt time.Time `json:"end_at"`

	// Status 活动状态（active / disabled）。
	Status string `json:"status"`
}

// SeckillDetail 在列表字段之上补充盲盒描述（详情页展示用）。
type SeckillDetail struct {
	SeckillListItem

	// Description 盲盒描述。
	Description string `json:"description"`
}

// SeckillBrowseService 是 C 端秒杀活动浏览接口。
type SeckillBrowseService interface {
	// List 拉当前生效的秒杀活动列表（分页，start_at ASC）。
	List(c *gin.Context, opts repository.ListOptions) ([]SeckillListItem, int64, error)

	// Detail 拉单个秒杀活动详情（含实时剩余名额）。
	Detail(c *gin.Context, seckillID int64) (*SeckillDetail, error)
}

// SeckillBrowseServiceDeps 注入 SeckillBrowseService 所需的依赖。
type SeckillBrowseServiceDeps struct {
	SeckillRepo  mallRepo.SeckillRepository
	BlindBoxRepo mallRepo.BlindBoxRepository
	SupplierRepo supplierRepo.SupplierRepository
	Cache        cache.Cache // 读实时剩余名额；nil 时全部走 DB 兜底（单测 / 缺依赖）
}

// baseSeckillBrowseService 是 SeckillBrowseService 的默认实现。
type baseSeckillBrowseService struct {
	seckill mallRepo.SeckillRepository
	bb      mallRepo.BlindBoxRepository
	sup     supplierRepo.SupplierRepository
	cache   cache.Cache
}

// NewSeckillBrowseService 接收依赖，返回 SeckillBrowseService 接口。
func NewSeckillBrowseService(d SeckillBrowseServiceDeps) SeckillBrowseService {
	return &baseSeckillBrowseService{
		seckill: d.SeckillRepo,
		bb:      d.BlindBoxRepo,
		sup:     d.SupplierRepo,
		cache:   d.Cache,
	}
}

// List 实现见 SeckillBrowseService 注释。
//
// 实现要点：
//  1. repo.ListActive 走 (status, start_at, end_at) 索引拿「当前生效」活动；
//  2. 批量查盲盒 + 供应商，剔除下架 / 禁用的（total 为过滤前总数，与盲盒列表同口径）；
//  3. 批量取剩余名额（Redis 优先 + DB 兜底）。
func (s *baseSeckillBrowseService) List(c *gin.Context, opts repository.ListOptions) ([]SeckillListItem, int64, error) {
	secs, total, err := s.seckill.ListActive(c, time.Now(), opts)
	if err != nil {
		return nil, 0, err
	}
	if len(secs) == 0 {
		return []SeckillListItem{}, total, nil
	}

	// 批量查盲盒，过滤下架 / 非启用。
	bbIDs := make([]int64, 0, len(secs))
	for _, sec := range secs {
		bbIDs = append(bbIDs, sec.BlindBoxID)
	}
	boxes, err := s.bb.ListByIDs(c, bbIDs)
	if err != nil {
		return nil, 0, err
	}
	bbMap := make(map[int64]*mallModel.MallBlindBox, len(boxes))
	for i := range boxes {
		bbMap[boxes[i].ID] = &boxes[i]
	}

	kept := make([]*mallModel.MallSeckillActivity, 0, len(secs))
	supplierIDs := make([]int64, 0, len(secs))
	for i := range secs {
		sec := &secs[i]
		bb := bbMap[sec.BlindBoxID]
		if bb == nil || bb.Status != mallModel.StatusActive || !bb.OnSale {
			continue
		}
		kept = append(kept, sec)
		supplierIDs = append(supplierIDs, bb.SupplierID)
	}

	// 批量查供应商，过滤已禁用。
	supMap := make(map[int64]*mallModel.MallSupplier, len(supplierIDs))
	if len(supplierIDs) > 0 {
		suppliers, err := s.sup.ListByIDs(c, supplierIDs)
		if err != nil {
			return nil, 0, err
		}
		for i := range suppliers {
			supMap[suppliers[i].ID] = &suppliers[i]
		}
	}

	visible := make([]*mallModel.MallSeckillActivity, 0, len(kept))
	for _, sec := range kept {
		bb := bbMap[sec.BlindBoxID]
		sup := supMap[bb.SupplierID]
		if sup == nil || sup.Status == mallModel.StatusDisabled {
			continue
		}
		visible = append(visible, sec)
	}

	remaining := s.remainingStocks(c, visible)
	items := make([]SeckillListItem, 0, len(visible))
	for _, sec := range visible {
		items = append(items, buildListItem(sec, bbMap[sec.BlindBoxID], supMap[bbMap[sec.BlindBoxID].SupplierID], remaining[sec.ID]))
	}
	return items, total, nil
}

// Detail 实现见 SeckillBrowseService 注释。
//
// 不存在 / 已禁用 / 关联盲盒或供应商不可购 → ErrSeckillNotAvailable（handler 404）。
// 时间窗外仍可查看（前端展示"未开始 / 已结束"状态），抽卡由 DrawSeckill 校验窗口。
func (s *baseSeckillBrowseService) Detail(c *gin.Context, seckillID int64) (*SeckillDetail, error) {
	if seckillID <= 0 {
		return nil, ErrSeckillNotAvailable
	}

	sec, err := s.seckill.GetByID(c, seckillID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeckillNotAvailable
		}
		return nil, err
	}
	if sec == nil || sec.Status != mallModel.StatusActive {
		return nil, ErrSeckillNotAvailable
	}

	bb, err := s.bb.GetByID(c, sec.BlindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeckillNotAvailable
		}
		return nil, err
	}
	if bb == nil || bb.Status != mallModel.StatusActive || !bb.OnSale {
		return nil, ErrSeckillNotAvailable
	}

	sup, err := s.sup.GetByID(c, bb.SupplierID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSeckillNotAvailable
		}
		return nil, err
	}
	if sup == nil || sup.Status == mallModel.StatusDisabled {
		return nil, ErrSeckillNotAvailable
	}

	remaining := s.remainingStocks(c, []*mallModel.MallSeckillActivity{sec})
	item := buildListItem(sec, bb, sup, remaining[sec.ID])
	return &SeckillDetail{
		SeckillListItem: item,
		Description:     bb.Description,
	}, nil
}

// remainingStocks 批量取剩余名额：Redis GET 为主，miss / 不可用走 DB 兜底。
//
// 返回 map 保证覆盖 secs 里每个 ID；拿不到的给 remainingUnknown（-1）。
func (s *baseSeckillBrowseService) remainingStocks(c *gin.Context, secs []*mallModel.MallSeckillActivity) map[int64]int {
	out := make(map[int64]int, len(secs))
	missIDs := make([]int64, 0, len(secs))

	for _, sec := range secs {
		out[sec.ID] = remainingUnknown
		if s.cache == nil {
			missIDs = append(missIDs, sec.ID)
			continue
		}
		v, found, err := s.cache.Get(c.Request.Context(), seckillRedisStockKey+strconv.FormatInt(sec.ID, 10))
		// 秒杀库存 key 不走 notFound 占位（D5.1 只覆盖详情/feed 两类只读聚合）；
		// !found（理论不可达）按 miss 处理走 DB 兜底。
		if err != nil || !found {
			missIDs = append(missIDs, sec.ID)
			continue
		}
		if n, perr := strconv.Atoi(strings.TrimSpace(v)); perr == nil && n >= 0 {
			out[sec.ID] = n
		} else {
			missIDs = append(missIDs, sec.ID)
		}
	}

	if len(missIDs) == 0 {
		return out
	}
	counts, err := s.seckill.CountDeductionLogsBySeckillIDs(c, missIDs)
	if err != nil {
		// DB 兜底也失败 → 保持 -1，浏览不被阻断。
		return out
	}
	for _, sec := range secs {
		if out[sec.ID] != remainingUnknown {
			continue
		}
		n := sec.TotalStock - int(counts[sec.ID])
		if n < 0 {
			n = 0
		}
		out[sec.ID] = n
	}
	return out
}

// buildListItem 把活动 + 盲盒 + 供应商组装成展示投影。
func buildListItem(sec *mallModel.MallSeckillActivity, bb *mallModel.MallBlindBox, sup *mallModel.MallSupplier, remaining int) SeckillListItem {
	return SeckillListItem{
		ID:             sec.ID,
		BlindBoxID:     sec.BlindBoxID,
		BlindBoxName:   bb.Name,
		BlindBoxCover:  bb.Cover,
		SupplierID:     bb.SupplierID,
		SupplierName:   sup.Name,
		OriginalPrice:  bb.Price,
		SeckillPrice:   sec.SeckillPrice,
		TotalStock:     sec.TotalStock,
		RemainingStock: remaining,
		PerUserLimit:   sec.PerUserLimit,
		StartAt:        sec.StartAt,
		EndAt:          sec.EndAt,
		Status:         string(sec.Status),
	}
}

// 编译期断言：baseSeckillBrowseService 必须实现 SeckillBrowseService。
var _ SeckillBrowseService = (*baseSeckillBrowseService)(nil)
