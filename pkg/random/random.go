// Package random 提供随机数生成与随机颜色工具。
//
// 设计原则：
//   - 依赖极少（math/rand + math + image/color），可被任意层引用；
//   - 与业务无关，不感知 captcha / 配置等概念。
package random

import (
	"image/color"
	"math"
	"math/rand"
	"time"
)

// New 返回一个基于当前时间纳秒种子化的 *rand.Rand。
// 不使用全局 rand 源，避免并发场景下的 data race。
func New() *rand.Rand {
	return rand.New(rand.NewSource(time.Now().UnixNano()))
}

// Color 返回一个饱和度与亮度适中的随机 RGBA 颜色（HSL 转换）。
// 用途：图形验证码中的随机元素颜色，避免 OCR 简单识别。
func Color(rng *rand.Rand) color.RGBA {
	return HSLToRGBA(rng.Float64()*360, 0.7+rng.Float64()*0.2, 0.4+rng.Float64()*0.2)
}

// HSLToRGBA 把 HSL 颜色转换为 RGBA；h ∈ [0,360)，s/l ∈ [0,1]。
func HSLToRGBA(h, s, l float64) color.RGBA {
	c := (1 - math.Abs(2*l-1)) * s
	x := c * (1 - math.Abs(math.Mod(h/60, 2)-1))
	m := l - c/2
	var r, g, b float64
	switch {
	case h < 60:
		r, g, b = c, x, 0
	case h < 120:
		r, g, b = x, c, 0
	case h < 180:
		r, g, b = 0, c, x
	case h < 240:
		r, g, b = 0, x, c
	case h < 300:
		r, g, b = x, 0, c
	default:
		r, g, b = c, 0, x
	}
	return color.RGBA{
		R: uint8(ClampByte((r + m) * 255)),
		G: uint8(ClampByte((g + m) * 255)),
		B: uint8(ClampByte((b + m) * 255)),
		A: 255,
	}
}

// ClampByte 把 float64 限制到 [0, 255] 范围。
func ClampByte(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}
