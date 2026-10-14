/**
 * 滑块验证码的 mock 实现。
 *
 * 图像用内联 SVG 生成（data URI），避免往仓库塞二进制资源，也不依赖 DOM canvas。
 * 缺口位置记录在模块级 Map 中 —— 默认不参与判定（见 MOCK_SLIDER_STRICT），
 * 打开开关后可验证「失败复位、连续 3 次失败重建」两条分支。
 */

import type { SliderCreateResponse, SliderVerifyResponse } from '@/types'
import { MOCK_SLIDER_STRICT } from './constants'
import { mockError } from './error'

const WIDTH = 300
const HEIGHT = 150
const BLOCK = 46

/** key → 真实缺口横坐标。 */
const answers = new Map<string, number>()

function toDataUri(svg: string): string {
  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(svg)}`
}

function buildBackground(gapX: number, gapY: number): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${HEIGHT}" viewBox="0 0 ${WIDTH} ${HEIGHT}">
<defs>
<linearGradient id="bg" x1="0" y1="0" x2="1" y2="1">
<stop offset="0" stop-color="#1d222c"/><stop offset="1" stop-color="#33405a"/>
</linearGradient>
</defs>
<rect width="${WIDTH}" height="${HEIGHT}" fill="url(#bg)"/>
<circle cx="58" cy="42" r="34" fill="#ffffff" opacity="0.05"/>
<circle cx="236" cy="104" r="46" fill="#ffffff" opacity="0.04"/>
<circle cx="152" cy="24" r="20" fill="#ffffff" opacity="0.06"/>
<rect x="${gapX}" y="${gapY}" width="${BLOCK}" height="${BLOCK}" rx="8" fill="#0b0d12" opacity="0.92"/>
<rect x="${gapX}" y="${gapY}" width="${BLOCK}" height="${BLOCK}" rx="8" fill="none" stroke="#f5b544" stroke-opacity="0.55" stroke-width="1.5"/>
</svg>`
  return toDataUri(svg)
}

function buildBlock(): string {
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="${BLOCK}" height="${BLOCK}" viewBox="0 0 ${BLOCK} ${BLOCK}">
<rect width="${BLOCK}" height="${BLOCK}" rx="8" fill="#f5b544" fill-opacity="0.92"/>
<path d="M${BLOCK * 0.28} ${BLOCK * 0.52}l${BLOCK * 0.16} ${BLOCK * 0.18} ${BLOCK * 0.3}-${BLOCK * 0.36}" fill="none" stroke="#0b0d12" stroke-width="3" stroke-linecap="round" stroke-linejoin="round"/>
</svg>`
  return toDataUri(svg)
}

export function mockCreateSlider(): SliderCreateResponse {
  const gapX = 90 + Math.floor(Math.random() * 120)
  const gapY = 20 + Math.floor(Math.random() * (HEIGHT - BLOCK - 40))
  const key = `mock-slider-${Date.now()}-${Math.floor(Math.random() * 10000)}`
  answers.set(key, gapX)
  return {
    key,
    image: buildBackground(gapX, gapY),
    block: buildBlock(),
    w: WIDTH,
    h: HEIGHT,
    y: gapY,
    block_w: BLOCK,
    block_h: BLOCK,
    expires_in: 300,
  }
}

export function mockVerifySlider(data?: Record<string, unknown>): SliderVerifyResponse {
  const key = String(data?.key ?? '')
  const x = Number(data?.x ?? -1)
  const w = Number(data?.w ?? 0)
  const h = Number(data?.h ?? 0)

  if (!key || !Number.isFinite(x) || x < 0) {
    throw mockError(400, 'captcha.invalid_input', '参数不完整')
  }
  if (w !== WIDTH || h !== HEIGHT) {
    throw mockError(400, 'captcha.invalid_input', '尺寸与创建时不一致')
  }
  if (!answers.has(key)) {
    throw mockError(410, 'captcha.expired', '验证码已过期，请重新验证')
  }
  // 预检语义：默认无条件通过（与 spec 一致）。严格模式用于本地验证失败分支。
  if (MOCK_SLIDER_STRICT) {
    const target = answers.get(key) as number
    if (Math.abs(x - target) > 8) {
      throw mockError(401, 'captcha.failed', '验证失败，请重试')
    }
  }
  return { code: 'captcha.ok', message: 'verified' }
}
