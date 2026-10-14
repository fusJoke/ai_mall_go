/** 数值与时间格式化。 */

/** 金额格式化：统一两位小数。`balance` 后端已是字符串，`price` 是数字。 */
export function formatPrice(value: number | string | null | undefined): string {
  if (value === null || value === undefined || value === '') return '0.00'
  const num = typeof value === 'string' ? Number(value) : value
  if (Number.isNaN(num)) return '0.00'
  return num.toFixed(2)
}

/** 权重（同池合计 10000）→ 百分比字符串。 */
export function formatPercent(weight: number | null | undefined): string {
  const w = Number(weight || 0)
  if (!Number.isFinite(w)) return '0.00%'
  return `${((w / 10000) * 100).toFixed(2)}%`
}

/** 时间格式化：`YYYY-MM-DD HH:mm`。入参为后端 RFC3339 字符串。 */
export function formatTime(value?: string | null): string {
  if (!value) return '-'
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return '-'
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`
}
