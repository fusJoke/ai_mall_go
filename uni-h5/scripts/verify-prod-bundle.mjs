#!/usr/bin/env node
/**
 * 生产产物凭证扫描（OCR 审查 P2-4 的第三道防线）。
 *
 * ## 职责边界（重要，别指望它做它做不到的事）
 *
 * 本脚本只做一件事：**在已构建的产物里找「长得像真实访问令牌」的字符串**。
 * 它**不**检测 `VITE_USE_MOCK=true` —— 因为 Vite 会把
 * `import.meta.env.VITE_USE_MOCK === 'true'` 编译成布尔字面量（`!0`），
 * 事后按值 grep 根本认不出来。mock 开关的拦截在**构建期**完成：
 * 见 `vite.config.ts` 的 `assertProdBuildHygiene()`。
 *
 * 三道防线各管一段：
 *   1. 构建期（vite.config.ts）—— 阻止「用带开发配置的 env 打包」，最权威；
 *   2. 运行期（src/config/index.ts 的 assertProductionHygiene）—— 兜住绕过 #1 的构建；
 *   3. 本脚本 —— 产物落地后的最后一道，抓凭证明文字面量（含源码里硬编码的情况）。
 *
 * ## 为什么判定用「形状」而不是「具体值」
 *
 * 令牌值可能来自 shell / CI secret 等任意来源，本脚本读不到；而且源码里硬编码的
 * 令牌同样要抓。后端签发的访问令牌是 **UUID v7**（见 cmd/devtoken、user/login 链路），
 * v7 的版本位固定为 `7`、变体位为 `8/9/a/b`，形状足够特殊 —— 同时 UUID v4（第三段
 * 以 `4` 开头）不会被误伤，而 v4 是本项目服务端唯一在用的其它 uuid 形态。
 *
 * 已知取舍：第三方库若把 UUID v7 用作**非密钥**用途（trace id / request id），
 * 会被误报。这里选择「宁可误报也不放过」—— 误报的代价是人工看一眼，
 * 漏报的代价是令牌已经躺在公网产物里。
 *
 * 用法：
 *     npm run build:h5:prod     # 先出生产包
 *     npm run verify:prod-bundle
 *
 * 退出码：0 = 通过；1 = 命中；2 = 无法完成扫描（目录不存在 / 为空 / 读取失败）。
 */

import { lstatSync, readdirSync, readFileSync } from 'node:fs'
import { extname, join } from 'node:path'
import process from 'node:process'

const DIST_DIR = join(process.cwd(), 'dist', 'build', 'h5')

/** 待扫标记。每条都必须能给出「为什么这个形状代表泄漏」的确定解释。 */
const MARKERS = [
  {
    name: '疑似真实访问令牌（UUID v7 形态）',
    // 形如 01a13a49-9e6c-799e-b752-cd86750de281。
    // 令牌只可能来自 VITE_DEV_TOKEN，而它不该出现在生产产物里。
    re: /\b[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b/g,
    hint:
      '把 VITE_DEV_TOKEN 从 .env / .env.local 移到 .env.development.local（仅 dev 加载 + 已 gitignore），' +
      '并轮换这枚令牌（它已经进过产物就视为已泄漏）。',
  },
]

/**
 * 按扩展名跳过无需扫描的文件。
 *
 * 两类：
 *   - `.map`：不影响运行期安全，且体积大；
 *   - 二进制资源：Vite 会把图片/字体/媒体原样拷进产物。令牌不可能存在于二进制里，
 *     而对这些文件做 `readFileSync(..., 'utf8')` 既浪费内存、又会产出无意义的
 *     解码字符串（未来若新增宽松的文本类标记，还会引入误报）。
 */
const SKIP_EXT = new Set([
  '.map',
  '.png',
  '.jpg',
  '.jpeg',
  '.gif',
  '.webp',
  '.avif',
  '.ico',
  '.bmp',
  '.svgz',
  '.woff',
  '.woff2',
  '.ttf',
  '.otf',
  '.eot',
  '.mp3',
  '.mp4',
  '.webm',
  '.mov',
  '.wasm',
  '.pdf',
  '.zip',
])

/**
 * 递归收集待扫产物文件。
 *
 * 用 `lstatSync` 而不是 `statSync`：后者会跟随符号链接，于是
 *   - 软链成环时 `walk` 无限递归、CI 直接挂住；
 *   - 无法区分「真文件」与「指向别处的软链」。
 * Vite 产物本来不该有软链，所以直接跳过即可，不必处理。
 */
function walk(dir) {
  const out = []
  for (const name of readdirSync(dir)) {
    const full = join(dir, name)
    const st = lstatSync(full)
    if (st.isSymbolicLink()) continue
    if (st.isDirectory()) out.push(...walk(full))
    else if (!SKIP_EXT.has(extname(name).toLowerCase())) out.push(full)
  }
  return out
}

/**
 * 掩码化令牌，只保留首尾各 4 个字符。
 *
 * 失败信息通常会被 CI 落盘或贴进 issue。把命中的令牌原样打出来，等于在
 * 「日志读取者」面前又泄露一次；而完全不打又无法定位是哪一枚。
 * 首尾各 4 位（合计 8 个 hex）足以在几枚令牌之间区分，且不构成可爆破的强度。
 */
function redact(token) {
  if (token.length <= 12) return '****'
  return `${token.slice(0, 4)}…${token.slice(-4)}`
}

function main() {
  let files
  try {
    files = walk(DIST_DIR)
  } catch (err) {
    // 不能把所有失败都归因成「目录不存在」：EACCES / ENOTDIR 的处置方式完全不同。
    console.error(`[verify:prod-bundle] 无法扫描产物目录：${DIST_DIR}`)
    console.error(`  失败原因：${err && err.code ? err.code : String(err)}`)
    if (err && err.message) console.error(`  详情：${err.message}`)
    console.error('  若目录确实尚未生成，请先执行 `npm run build:h5:prod`（这会顺带跑构建期门禁）。')
    process.exit(2)
  }

  if (files.length === 0) {
    // 「扫了 0 个文件然后报 OK」正是本脚本最该防的失效模式：
    // 构建中途失败只建出了空目录、或部署脚本指错了路径时，
    // 校验会变成一次毫无意义的绿灯。
    console.error(`[verify:prod-bundle] 产物目录为空：${DIST_DIR}`)
    console.error('  没有任何文件可扫描 —— 校验视为失败，而不是通过。')
    console.error('  请确认 `npm run build:h5:prod` 真的产出了产物。')
    process.exit(2)
  }

  const hits = []
  for (const file of files) {
    const text = readFileSync(file, 'utf8')
    for (const marker of MARKERS) {
      // 不用手动重置 lastIndex：`String.prototype.match` 遇到带 g 的正则时，
      // 会走 RegExp[Symbol.match]，它在匹配前后都会把 lastIndex 归零。
      const found = text.match(marker.re)
      if (found) {
        hits.push({ file, marker: marker.name, samples: [...new Set(found)].slice(0, 3), hint: marker.hint })
      }
    }
  }

  if (hits.length === 0) {
    console.log(
      `[verify:prod-bundle] OK —— 已扫描 ${files.length} 个产物文件，未发现凭证泄漏。\n` +
        '  注意：本脚本不检测 VITE_USE_MOCK（内联后不可辨），该项由构建期门禁保证。'
    )
    return
  }

  console.error(`[verify:prod-bundle] FAIL —— 生产产物中发现 ${hits.length} 处问题：`)
  for (const h of hits) {
    console.error(`  · ${h.marker}`)
    console.error(`    文件：${h.file}`)
    console.error(`    命中：${h.samples.map(redact).join(' , ')}`)
    console.error(`    处置：${h.hint}`)
  }
  process.exit(1)
}

main()
