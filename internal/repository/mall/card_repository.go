// Package mall 是 mall 业务域的数据访问层（task 2.3 落地范围）。
//
// 本包按实体拆文件，一个文件一个仓储：
//   - card_repository.go        —— MallCard
//   - blindbox_repository.go    —— MallBlindBox
//   - cardpool_repository.go    —— MallCardPool + MallCardPoolItem
//   - promotion_repository.go   —— MallPromotion
//   - draw_order_repository.go  —— MallDrawOrder + MallDrawOrderItem
//
// 后续 Phase 8 加 settlement_repository.go，Phase 9 加 seckill_repository.go。
package mall

import (
	"github.com/gin-gonic/gin"

	"ai-go-mall/internal/model/mall"
	"ai-go-mall/internal/repository"
)

// CardRepository 定义 mall_cards 表的数据访问接口。
//
// 设计原则：
//   - 通用 CRUD 由 CRUDRepository[mall.MallCard] 提供；
//   - 业务查询（ListFeatured / ListByIDs / DecrementStock）走 GORM 链；
//   - MallCard 是「卡牌主数据」：无 stock / rarity 字段，相关属性在 pool_items。
type CardRepository interface {
	repository.CRUDRepository[mall.MallCard]

	// GetByID 按主键查单行。
	GetByID(c *gin.Context, id int64) (*mall.MallCard, error)

	// ListByIDs 按 id 集合批量取卡牌行。
	ListByIDs(c *gin.Context, ids []int64) ([]mall.MallCard, error)

	// ListByTeam 按球队批量取卡牌行（首页 feed 球队分类场景）。
	ListByTeam(c *gin.Context, team string, limit int) ([]mall.MallCard, error)
}

// gormCardRepository 是 CardRepository 的 *gorm.DB 实现。
type gormCardRepository struct {
	repository.CRUDRepository[mall.MallCard]
}

// NewCardRepository 返回 CardRepository 接口。
func NewCardRepository() CardRepository {
	return &gormCardRepository{
		CRUDRepository: repository.NewBaseRepository[mall.MallCard](),
	}
}

// GetByID 按主键查单行。
func (r *gormCardRepository) GetByID(c *gin.Context, id int64) (*mall.MallCard, error) {
	var card mall.MallCard
	if err := repository.DB(c).First(&card, id).Error; err != nil {
		return nil, err
	}
	return &card, nil
}

// ListByIDs 按 id 集合批量取卡牌行；空 ids 返回空切片。
func (r *gormCardRepository) ListByIDs(c *gin.Context, ids []int64) ([]mall.MallCard, error) {
	rows := make([]mall.MallCard, 0)
	if len(ids) == 0 {
		return rows, nil
	}
	err := repository.DB(c).Where("id IN ?", ids).Find(&rows).Error
	return rows, err
}

// ListByTeam 按球队批量取卡牌行。
func (r *gormCardRepository) ListByTeam(c *gin.Context, team string, limit int) ([]mall.MallCard, error) {
	rows := make([]mall.MallCard, 0)
	q := repository.DB(c).Where("team = ?", team)
	if limit > 0 {
		q = q.Limit(limit)
	}
	err := q.Find(&rows).Error
	return rows, err
}

// 编译期断言。
var _ CardRepository = (*gormCardRepository)(nil)