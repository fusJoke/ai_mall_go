# user-draw-result Specification

## Purpose
为 C 端抽卡/秒杀抽卡提供嗨卡式「开盒仪式感」结果页，并把「抽卡 → 看结果 → 查订单」与「游客 → 登录 → 回跳原页」两条动线串联，使核心购买流端到端无断点。

## Requirements

### Requirement: 开卡结果页展示本次抽卡结果

系统 SHALL 提供 `/user/draw/result` 路由页面，从 sessionStorage 读取最近一次抽卡结果（`order_no` / `actual_price` / `cards[]`）并渲染：成功氛围区（开盒成功文案）、卡牌展示网格（每张卡显示 `snapshot_image` 卡面、稀有度边框色、`snapshot_name` 卡名）、订单信息条（订单号 + 实付金额）、操作区（查看订单 / 再抽一单 / 回首页）。稀有度边框色 SHALL 至少区分 SSR / SR / R / N 四档。

#### Scenario: 普通抽卡成功进入结果页

- **WHEN** 用户在盲盒详情页点击抽卡且接口返回 `DrawResult`
- **THEN** 结果写入 sessionStorage 后跳转 `/user/draw/result`，页面展示返回的全部卡牌（数量与 `cards.length` 一致）及订单号、实付金额，不再显示详情页内的文字成功提示

#### Scenario: 秒杀抽卡共用同一结果页

- **WHEN** 用户在秒杀详情页秒杀成功且接口返回 `SeckillDrawResult`
- **THEN** 同样跳转 `/user/draw/result`，渲染结构与普通抽卡完全一致

#### Scenario: 每张卡按稀有度着色

- **WHEN** 结果页渲染 `cards[]`
- **THEN** 每张卡显示卡面图（无图时显示占位）与卡名，且卡片边框色按稀有度区分：SSR 金、SR 紫、R 蓝、N 灰

### Requirement: 结果页操作区提供三条动线出口

结果页 SHALL 提供操作区：「查看订单」跳转 `/user/orders`；「再抽一单」跳转来源盲盒详情 `/user/blindbox/:id`（来源 id 随结果一并传递）；「回首页」跳转 `/user/home`。

#### Scenario: 查看订单

- **WHEN** 用户在结果页点击「查看订单」
- **THEN** 跳转至 `/user/orders` 订单列表

#### Scenario: 再抽一单

- **WHEN** 结果携带来源盲盒 id 且用户点击「再抽一单」
- **THEN** 跳转至 `/user/blindbox/:id` 盲盒详情页

### Requirement: 结果数据经 sessionStorage 传递并有刷新兜底

抽卡结果 SHALL 经 sessionStorage（固定 key）传递到结果页，读取成功后 SHALL 清除该 key 防止重复消费；结果页刷新或无数据直达时 SHALL NOT 白屏，自动重定向到 `/user/orders`。

#### Scenario: 刷新不白屏

- **WHEN** 用户在结果页按 F5 刷新（sessionStorage 中结果已被清除）
- **THEN** 页面自动跳转订单列表，不出现空白页或报错

#### Scenario: 未登录或手动直达

- **WHEN** 用户未经过抽卡直接访问 `/user/draw/result` 且 sessionStorage 无结果
- **THEN** 自动重定向到 `/user/orders`

### Requirement: 抽卡 401 引导登录并回跳原页面

盲盒详情与秒杀详情在用户未登录点击抽卡、或抽卡接口返回 401 时，SHALL 跳转 `/user/login?redirect=<当前页 fullPath>`；登录页 SHALL 读取 `redirect` 查询参数，登录成功后回跳该站内路径（仅接受以 `/user` 开头的路径，其余回退首页）。

#### Scenario: 游客抽卡后登录回跳详情

- **WHEN** 游客在 `/user/blindbox/3` 点击抽卡并在登录页完成登录
- **THEN** 登录成功后回到 `/user/blindbox/3`，可再次点击抽卡

#### Scenario: 拒绝站外跳转

- **WHEN** `redirect` 参数为外部 URL（如 `https://evil.example`）或非 `/user` 前缀路径
- **THEN** 登录成功后回退到 `/user/home`，不跳转外部地址
