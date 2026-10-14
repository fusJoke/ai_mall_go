/**
 * 稀有度工具。
 *
 * 后端枚举：SSR / SR / R / N（`internal/model/mall/card_pool.go`）。
 * 颜色一律通过 CSS 变量引用，禁止在此处出现十六进制色值。
 */

export const RARITY_ORDER = ['SSR', 'SR', 'R', 'N'] as const

export type Rarity = (typeof RARITY_ORDER)[number] | string

const VAR_MAP: Record<string, string> = {
  SSR: '--ssr',
  SR: '--sr',
  R: '--r',
  N: '--n',
}

/** 稀有度 → CSS 变量名；未知值回退 `--n`。 */
export function rarityVar(rarity?: string | null): string {
  const key = String(rarity || '').toUpperCase()
  return VAR_MAP[key] || '--n'
}

/** 稀有度 → 可直接用于 style 绑定的颜色表达式。 */
export function rarityColor(rarity?: string | null): string {
  return `var(${rarityVar(rarity)})`
}

/** 稀有度排序权重，用于分组展示。 */
export function rarityIndex(rarity?: string | null): number {
  const idx = RARITY_ORDER.indexOf(String(rarity || '').toUpperCase() as (typeof RARITY_ORDER)[number])
  return idx === -1 ? RARITY_ORDER.length : idx
}

/** 判断是否为高稀有度（用于结果页放大展示）。 */
export function isHighRarity(rarity?: string | null): boolean {
  return ['SSR', 'SR'].includes(String(rarity || '').toUpperCase())
}
