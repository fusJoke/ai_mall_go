// Package draw_check — blindbox_buyable_check.go 实现「盲盒是否可买」校验节点。
//
// 校验内容（spec Requirement "Draw transaction atomicity" 前置校验 #1 + #2）：
//
//	mall_blind_boxes.status = active AND on_sale = true
//	mall_suppliers.status   = active
//
// 失败出口：chain.ErrBlindBoxNotAvailable（handler 映射 HTTP 404 draw.blindbox_not_available）。
//
// 设计要点：
//   - 一次 GetByID 拉盲盒，命中后再 GetByID 拉供应商（防御 admin 禁用供应商后还能抽）；
//   - 「不存在 / 已下架 / 供应商禁用」统一用 ErrBlindBoxNotAvailable，不暴露存在性；
//   - seckill 链路要求 chain_builder 已经在调用本 check 前把 input.BlindBoxID
//     从 seckill 表里解析出来（见 chain_builder），所以本节点不感知 source 差异。
package draw_check

import (
	"errors"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"ai-go-mall/internal/domain/chain"
	mallModel "ai-go-mall/internal/model/mall"
	mallRepo "ai-go-mall/internal/repository/mall"
	supplierRepo "ai-go-mall/internal/repository/supplier"
)

// blindboxBuyableCheck 校验盲盒可买（status=active + on_sale=true + supplier 启用）。
type blindboxBuyableCheck struct {
	bb        mallRepo.BlindBoxRepository
	suppliers supplierRepo.SupplierRepository
}

// NewBlindBoxBuyableCheck 构造 blindboxBuyableCheck Validator。
func NewBlindBoxBuyableCheck(bb mallRepo.BlindBoxRepository, suppliers supplierRepo.SupplierRepository) chain.Validator {
	return &blindboxBuyableCheck{bb: bb, suppliers: suppliers}
}

// Name 实现 chain.Validator。
func (c *blindboxBuyableCheck) Name() string { return "blindbox_buyable" }

// Validate 读盲盒 + 供应商表，校验可买。
//
// 通过：return nil。
// 拒绝：return chain.ErrBlindBoxNotAvailable（wrap 由 Chain.Validate 完成）。
//
// 注：本节点假定 input.BlindBoxID > 0；chain_builder 在组装 DrawChain 时
// 已经过滤 BlindBoxID=0 的输入（Chain.Validate 入口有 ErrMissingInput 兜底）。
func (c *blindboxBuyableCheck) Validate(ctx *gin.Context, input *chain.DrawInput) error {
	if input.BlindBoxID <= 0 {
		return chain.ErrBlindBoxNotAvailable
	}
	bb, err := c.bb.GetByID(ctx, input.BlindBoxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return chain.ErrBlindBoxNotAvailable
		}
		return err
	}
	if bb == nil || bb.Status != mallModel.StatusActive || !bb.OnSale {
		return chain.ErrBlindBoxNotAvailable
	}

	// 防御 admin 禁用供应商后还能抽：status=disabled → ErrBlindBoxNotAvailable。
	sup, err := c.suppliers.GetByID(ctx, bb.SupplierID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return chain.ErrBlindBoxNotAvailable
		}
		return err
	}
	if sup == nil || sup.Status == mallModel.StatusDisabled {
		return chain.ErrBlindBoxNotAvailable
	}
	return nil
}

// 编译期断言。
var _ chain.Validator = (*blindboxBuyableCheck)(nil)
