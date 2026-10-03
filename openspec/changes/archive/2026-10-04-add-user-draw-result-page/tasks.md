# Tasks: C 端开卡结果页与抽卡主流程串联

## 1. 开卡结果页

- [x] 1.1 新增 `web/src/views/user/draw/result.vue`：sessionStorage 读取结果 → 氛围区 / 卡牌网格（稀有度边框色）/ 订单信息条 / 操作区；读取后清 key，无数据 `replace('/user/orders')` 兜底
- [x] 1.2 新增路由 `userDrawResult`：`router/static/userBase.ts` 注册 `/user/draw/result`（置于 `:path(.*)*` 兜底之前）
- [x] 1.3 新增 i18n：`lang/{zh-cn,en}/user.yaml` 增加 `drawResult.*` 文案；`pageTitles.yaml` 增加 `userDrawResult`

## 2. 抽卡动线改造

- [x] 2.1 修改 `views/user/blindbox/detail.vue`：`onDraw` 成功分支改为 sessionStorage 写入 + `router.push('/user/draw/result')`，移除文字成功提示
- [x] 2.2 修改 `views/user/seckill/detail.vue`：`onDraw` 成功分支同样跳结果页（保留跳转前刷新剩余名额逻辑）

## 3. 登录回跳

- [x] 3.1 修改 `views/user/blindbox/detail.vue`：未登录点抽卡 / 401 分支改为 `router.push({ path: '/user/login', query: { redirect: route.fullPath } })`（关注按钮 401 分支同改）
- [x] 3.2 修改 `views/user/seckill/detail.vue`：未登录 / 401 分支同上
- [x] 3.3 修改 `views/user/login.vue`：登录成功读取 `route.query.redirect`，仅接受 `/user` 开头的站内路径，否则回 `/user/home`

## 4. 验证与归档

- [x] 4.1 `npm run typecheck` 与 `npm run build` 通过
- [x] 4.2 本地起前后端，GUI 冒烟：登录 → 详情 → 抽卡 → 结果页（卡面/稀有度色/订单信息）→ 查看订单；游客 401 → 登录 → 回跳详情；结果页刷新兜底
- [x] 4.3 `openspec validate add-user-draw-result-page` 通过后归档
