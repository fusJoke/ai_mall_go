# Proposal: C 端开卡结果页与抽卡主流程串联

## Why

参照嗨卡 app 的核心体验「线上即时开盒」，抽卡结果的**仪式感呈现**是主流程的关键一环。当前 C 端抽卡成功后仅在盲盒详情页内显示一条拼接文字提示（`show('success', ...)`），卡面图、稀有度、订单号等信息挤在一行小字里，开盒仪式感完全缺失；同时游客在详情页点「抽卡」跳登录后一律落到首页，丢失了原页面上下文，浏览动线被打断。

依据产品规划 `C端页面规划与主流程.md` 与 `C端线框图.html` 的 P0 范围，本 change 打通核心购买流：

> 登录 → 首页 → 盲盒详情（概率公示）→ 抽卡 → **开卡结果页★** → 查看订单 → 订单详情

## What Changes

### 前端（本次全部改动，零后端改动）

- 新增 `views/user/draw/result.vue` 开卡结果页 `/user/draw/result`：
  - 成功氛围区（开盒成功文案 + 稀有度色调）
  - 卡牌展示网格：`snapshot_image` 卡面 + 稀有度边框色（SSR 金 / SR 紫 / R 蓝 / N 灰）+ 卡名
  - 订单信息条：`order_no` + `actual_price`
  - 操作区：查看订单 → 订单列表 / 再抽一单 → 回盲盒详情 / 回首页
  - 结果经 sessionStorage 传递；无数据时自动跳订单列表（防刷新丢参白屏）
- 修改 `views/user/blindbox/detail.vue`：抽卡成功由文字提示改为写入 sessionStorage 后 `router.push('/user/draw/result')`
- 修改 `views/user/seckill/detail.vue`：秒杀抽卡成功同样跳开卡结果页（`SeckillDrawResult` 与 `DrawResult` 结构一致，共用本页）；保留原有剩余名额刷新逻辑于跳转前
- 修改盲盒详情 / 秒杀详情的未登录与 401 分支：跳转 `/user/login?redirect=<当前 fullPath>`，登录成功回跳原页面
- 修改 `views/user/login.vue`：读取 `route.query.redirect`，登录成功回跳该地址（仅接受站内 `/user` 前缀路径，防开放跳转）
- 新增路由：`router/static/userBase.ts` 增加 `userDrawResult` 路由
- 新增 i18n：`lang/{zh-cn,en}/user.yaml` 增加 `user.drawResult.*`；`pageTitles.yaml` 增加 `userDrawResult`

### 不改

- 后端任何代码（`DrawResult.cards[]` 已含 snapshot_image / rarity / snapshot_name / order_no / actual_price，全部现成）
- 订单列表 / 订单详情页
- P1 范围（底部导航、我的页、盲盒列表页、订单空态引导）与 P2 卡册——列入后续 change

## Capabilities

### New Capabilities

- `user-draw-result`：C 端开卡结果页——抽卡/秒杀结果仪式感展示、sessionStorage 传递与刷新兜底、登录回跳串联。

### Modified Capabilities

无——`trading-card-blindbox-mvp` spec 的接口契约不变，本次纯前端呈现与动线调整。

## Impact

- 前端：1 个新页面 + 3 个页面修改 + 1 处路由 + 2 个 lang 文件，约 400 行
- 后端：0 改动
- 风险：低——新增页面独立，详情页仅改抽卡成功与 401 两个分支
