# Design

## Context

- `internal/infra/captcha/click.go` 已实现点选验证码核心：背景图+元素合成、碰撞检测、sha1 答案摘要落库、按 elements 序列的逻辑校验。已存在 `Manager.CreateClick / VerifyClick`，但 VerifyClick 当前只比对元素名序列，不带坐标精度。
- `internal/model/captcha.go` 与 `internal/repository/captcha/captcha.go` 完整；`captchas` 表已建（`database.Init()` 中的 AutoMigrate 已覆盖）。
- 后端路由层通过 `internal/router/registry` init() 自注册；当前 `internal/router/{admin,user,index}` 三组，没有 common 前缀。
- 前端 `web/src/components/clickCaptcha/` 有完整弹窗组件（命令式 API：`clickCaptcha(uuid, callback, options)`），但**与后端契约不一致**：
  - 后端响应 `{Key, Elements[], Image, Width, Height}`（Elements 是答案名数组）；前端期望 `{id, text, base64, width, height}`（字段名 + text 当字符串）
  - 后端 verify 入参 `{key, answer[]}`（元素名数组）；前端发 `{uuid, captchaInfo: "x,y-x,y;w;h", unset}`（坐标串）
  - 后端按元素名顺序比对；前端按点击坐标提交。两者直接对接不通。
- 前端 `login.vue` 当前不引用 clickCaptcha，`LoginRequest` 无 captchaKey 字段，`Login` service 不做二次校验。
- 当前 `infra/captcha.Manager` 的图片渲染固定 320x180（`bgWidth=320, bgHeight=180` 常量）。提案正文里的"350x200"以实际渲染尺寸为准，下游一致采用 320x180。

## Goals / Non-Goals

**Goals:**

- 把 `infra/captcha.Manager` 通过 handler/service/router 暴露成 HTTP 接口，契约对齐。
- 把精度校验做对：前端提交原始 320x180 坐标，后端按 bbox 中心 + 容差半径比对。
- 答错 M=3 强制刷新：失败计数内存维护，超阈值立即硬删 key。
- 预检不消耗 key、消费型 verify 一次性消费：保证登录链路"先预检通过再二次校验"的顺序安全。
- 登录链路顺序门控（verify → password → token），任一环节失败立即终止。
- 前端组件按新契约改造，`login.vue` 接入。

**Non-Goals:**

- 不引入新的 Go 模块或 npm 包（freedtype/image/png 已在用）。
- 不新增 yaml 配置项（M 与容差半径以 Go 常量固化）。
- 不动 `captchas` 表结构。
- 不做"答错 N 次锁定账号 / IP 限流"等更激进策略（仅 per-key 计数）。
- 不重写或拆分 `infra/captcha` 内部合成逻辑。
- 不为 captcha capability 增加除登录外的其它消费方（注册/找回密码留待后续 change）。

## Decisions

### D1. 路由分组：新增 `/common` 前缀

`internal/router/common/` 新建 `init()` 通过 `registry.Register("/common", ...)` 注册两条路由：

```
GET  /common/captcha/create  -> captcha handler CreateClick
POST /common/captcha/verify  -> captcha handler VerifyClick
```

**Why `/common` 而不是 `/captcha`**：与现有 `/admin` `/user` 按身份分组保持一致；`/common` 表达"跨身份共用能力"。

### D2. handler/service 分层

按项目惯用四层（handler → service → infra → repo）：

```
internal/handler/captcha/handler.go    # HTTP 入参解析、错误码映射
internal/service/captcha/service.go    # 包装 infra/captcha.Manager，注入 tokenManager 等（如有）
internal/router/common/common.go       # init() 自注册，组装 handler
```

service 层是薄的，主要是把 infra 包的方法翻译成"业务领域语言"（如错误归类、参数校验）。infra 层保持技术中立。

### D3. VerifyClick 入参形态与精度比对

入参改为：

```go
type VerifyClickReq struct {
    Key    string  `json:"key"`
    Points []Point `json:"points"`  // [{X, Y int}, ...]
    W      int     `json:"w"`
    H      int     `json:"h"`
}
type Point struct {
    X int `json:"x"`
    Y int `json:"y"`
}
```

比对逻辑：

```
1. 校验 W==320 && H==180，否则 ErrInvalidInput
2. 取出 captchas[key]，Info 字段存的是 sha1(elements) 的 hex
3. 按 elements[i] 的类型（icon vs text）查 elemSize：
   - icon: 44x44，容差 24px
   - text: 24x28，容差 14px
4. 已知元素放置时的中心 (cx_i, cy_i) 在当前实现里不持久化，只存了 answer 顺序
   → 必须改造 composeImage / placeElem 把每个正确答案的 (cx, cy) 一并写到 Info
   → Info JSON 形态：{"answer_sha1":"...","placed":[{"name":"A","cx":120,"cy":80,"size":"text"}, ...]}
5. 对 points[i] 与 placed[i].(cx,cy) 算欧氏距离，超过对应容差即 ErrMismatch
```

**Why 存 placed 坐标而非重做图像识别**：合成时已经知道 (cx, cy)，存下来最便宜；不存的话就得反向工程（OCR / 图像匹配），代价大且容易误判。

**考虑过的备选**：
- 只存元素名 + 区域 hash（图片按元素 bbox 分块哈希）：前端要提交每个点对应的元素名，等于把"点选"退化成"选名"，失去防脚本意义 → 否决。
- 前端把每个点对应元素名 + 坐标都提交：绕过 OCR 但仍需点对位置 → 实质等价于存坐标，省事不省力 → 不采用。

### D4. 答错计数存储

`infra/captcha.Manager.failCnt` 是 `sync.Map[string]*failEntry`，每个 `failEntry` 含 `*atomic.Int64` 计数器与该 key 对应的 `expiresAt`。`VerifyClick` 失败时 `recordFail` 调 `entry.cnt.Add(1)`，达到 `MaxFailPerKey` 立即 `DeleteByKey` 并从 `failCnt` 删除该项。

```
type failEntry struct {
    cnt       *atomic.Int64
    expiresAt time.Time
}
```

**Why 内存而非 DB**：
- key 本身是短生命周期（默认 600s 过期），过期清理会带走计数；
- M=3 防的是"同一个 key 被反复试错"——key 一删即终止攻击面，无需跨进程可见；
- 持久化反而引入"计数要持久化到什么表"的额外设计成本。

**过期回收**：`cleanupLazy` 每次 CreateClick / VerifyClick 触发时，**同时**清理 DB（`DeleteExpired`）与 `failCnt`（按 `expiresAt < now` 剔除）。最初实现只清 DB 不清 failCnt，导致 1-2 次失败后放任过期的 key 永久泄漏在 map 中，被 Codex review 指出后改为同步回收。

**并发安全**：计数用 `atomic.Int64` 而非 `*int`：`sync.Map` 只保证 map 自身的并发读写安全，不保护指针指向的值；用 `*int++` 在并发 recordFail 下会丢失更新（`go test -race` 必报），攻击者可借此让计数器涨不到 `MaxFailPerKey`，绕开 M=3 防刷。改用 `atomic.Int64.Add` 后 RMW 是原子的，丢失更新被消除。

**Trade-off**：
- 进程重启会让所有未达阈值的计数归零，攻击者可借此重置一次——key 600s 过期 + 单次会话最多 3 次机会，这个窗口可接受；
- `failCnt.Delete(key)` 与并发 `LoadOrStore` 之间存在竞态窗口：少数 goroutine 可能 delete 后又新建一个 cnt=1 的 entry。该 entry 单独无法达到 M=3，且其 `expiresAt` 会让 `cleanupLazy` 在下次懒清理时回收，不会无限堆积。

### D5. preCheck vs consumeCheck 的参数化

`VerifyClick(ctx, req, deleteOnSuccess bool)` 已经是变参接口，无需改签名：

```
预检场景（前端 onSubmit 之前）：  deleteOnSuccess=false
消费场景（登录 handler 内）：     deleteOnSuccess=true
```

调用方在调用点显式选择，框架不强制配对。handler 层不在 `/common/captcha/verify` 路由上区分两者——这条路由**专用于预检**（永远 deleteOnSuccess=false），登录内的二次校验走 service 直接调 infra，不暴露 HTTP。

**Why 不暴露"消费型 verify"HTTP 端点**：避免被外部脚本绕过登录链路直接调用 verify。二次校验必须是后端内部行为。

### D6. 登录顺序门控的注入位置

`service/admin.Login(c, username, password, captchaKey, remember)` 入参加 `captchaKey`。流程：

```
1. m.captcha.VerifyClick(c, &VerifyClickReq{Key: captchaKey, ...}, deleteOnSuccess=true)
   err != nil → return nil, "", ErrInvalidCaptcha
2. repo.GetByUsername(c, username)
   ErrRecordNotFound → return nil, "", ErrInvalidCredentials (与密码错误同文案)
3. bcrypt.CompareHashAndPassword(...)
   err != nil → LoginFailure++, Update; return ErrInvalidCredentials
4. adm.Status != 1 → return ErrAccountDisabled
5. token issue + update last_login
```

**Why captcha 校验最先**：用户原话"第 1 步的预检此时应被认为是不可靠的，不能因为有预验就直接执行登录接口内的密码验证等工作"。把 captcha 二次校验放在最前，密码错误计数就不会被恶意脚本无谓触发。

**考虑过的备选**：
- 用 Gin middleware 在 /admin/login 路由前做 captcha 校验：登录接口需要 username/password 才能定位 captcha 来源，且 middleware 与 service 之间要共享状态，复杂度更高 → 否决。

### D7. 前端组件改造方向

`web/src/components/clickCaptcha/`：

- index.vue `state.captcha` 改为 `{key, elements: [], image, width, height}`；
- `onRecord` 累计 `state.xy: [{x,y}, ...]`，达到 `elements.length` 后提交：
  ```
  POST /common/captcha/verify
  body: { key, points: state.xy, w: state.captcha.width, h: state.captcha.height }
  ```
- callback 透传 `captchaKey`（即 `state.captcha.key`），而非坐标串。

`web/src/views/admin/login.vue`：

- `onSubmit` 改为先调 `clickCaptcha(uuid, cb)`，`cb` 内拿到 `captchaKey` 后再调 `login(...)`；
- `login()` 请求体加 `captchaKey`。

**Why 弹窗式组件不需要改 UI 形态**：现有"fixed 居中弹窗 + 半透明遮罩"的 UI 与 design 系统兼容，改造集中在数据契约，不动 CSS。

### D8. 错误码统一

后端 `infra/captcha` 已有 `ErrInternal / ErrInvalidInput / ErrNotFound / ErrExpired / ErrInvalidElem / ErrMismatch`。handler 层把 `infra` error 映射为：

| infra error | HTTP | 业务错误码 |
| --- | --- | --- |
| ErrInvalidInput | 400 | `captcha.invalid_input` |
| ErrNotFound | 404 | `captcha.not_found` |
| ErrExpired | 410 | `captcha.expired` |
| ErrMismatch | 401 | `captcha.mismatch` |
| 其他 | 500 | `captcha.internal` |

**考虑过的备选**：直接用 `{code, message}` 信封包裹 → 当前后端不统一使用信封（看 login handler 是直接 `gin.H{"error": ...}`），保持一致。

## Risks / Trade-offs

- [Risk] 前端 onClick 的 `event.offsetX/offsetY` 与 CSS 缩放 / devicePixelRatio 不一致，导致真实坐标与图片原始 320x180 不匹配 → Mitigation：在 onRecord 内对 `<img>.naturalWidth` / `naturalHeight` 做比例归一化，把 offsetX/offsetY 映射回原始像素坐标；或要求 `<img>` 不做缩放（CSS `width: 350px` 改为 `width: 320px`）。由 tasks 中明确处理。
- [Risk] 内存答错计数在进程重启后归零，攻击者可借此重置 → Mitigation：key 600s 短 TTL，单进程单次会话最多 3 次机会，攻击窗口小；后续如需可改为 DB 计数。
- [Risk] placed 坐标随 Info 持久化后，DB 体积增加（每次 create 多写几个 int）→ Mitigation：单条 Info 几百字节量级，可忽略；将来可考虑移到 Redis。
- [Risk] `infra/captcha` 已有 `click_test.go`，改造 VerifyClick 形态会破坏现有测试 → Mitigation：tasks 中先重构测试再实现新行为。
- [Risk] 预检 + 二次校验的两步模式让前端代码复杂度上升 → Mitigation：组件封装好 callback，login.vue 只关心"拿到 captchaKey 后调 login"。

## Migration Plan

无破坏性 schema 变更，部署步骤：

1. **后端**：替换 `infra/captcha` 包 + 新增 handler/service/router 三个包，重启服务即可（向后兼容：旧前端仍能调通老接口，单纯是新前端用新契约）。
2. **前端**：灰度替换 clickCaptcha 组件；旧版 login.vue 在过渡期可保留，但应在登录请求里不带 captchaKey（被新后端拒绝 400）。
3. **回滚**：后端回滚到旧 commit 时，前端新组件会因契约不通 100% 失败 → 应**前端先行灰度**，确认全量切换后再升级后端。

## Open Questions

（无可安全延后的开放问题；D1~D8 已涵盖所有架构决策。）