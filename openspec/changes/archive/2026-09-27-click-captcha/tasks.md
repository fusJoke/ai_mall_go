# Tasks

## 1. 重构 infra/captcha 答案存储与精度校验

- [x] 1.1 改造 `composeImage` 让每个正确答案元素返回 `{name, cx, cy, kind}` 而不仅是 name，并新增常量 `ToleranceRadiusText=14, ToleranceRadiusIcon=24, ImageWidth=350, ImageHeight=200`（与 asset/captcha/click/background/*.png 一致），并验证 `bgWidth/bgHeight` 与 ImageWidth/ImageHeight 一致 —— `go build ./...` 通过
- [x] 1.2 改造 `Manager.CreateClick` 把 placed 坐标序列化为 JSON 写入 `captchas.Info`（形态：`{"sha":"<sha1-hex>","points":[{"name":"A","cx":120,"cy":80,"kind":"text"},...]}`），并验证 `click_test.go` 中 `TestCreateClick_ReturnsValidCaptcha` 仍然通过 —— `go test ./internal/infra/captcha/ -run TestCreateClick -v`
- [x] 1.3 改造 `VerifyClickReq` 为 `{Key, Points:[{X,Y}], W, H}`，新增精度比对逻辑：先校验 `W==350 && H==200`、长度匹配，再按 points[i] 与 placed[i] 算欧氏距离超过容差即 ErrMismatch，并验证新增 `TestVerifyClick_*` 覆盖通过/坐标偏离/尺寸不一致/空 key 场景 —— `go test ./internal/infra/captcha/ -v`
- [x] 1.4 在 `Manager` 上加 `failCnt sync.Map` 计数与常量 `MaxFailPerKey=3`：每次 verify 失败 `cnt++`，`cnt>=3` 立即 `DeleteByKey` 并清除计数；并验证新增 `TestVerifyClick_MaxFailTriggersDelete` 在第 3 次失败后 key 已被硬删 —— `go test ./internal/infra/captcha/ -v`

## 2. 后端 HTTP 层（handler / service / router）

- [x] 2.1 新建 `internal/handler/captcha/handler.go`，定义 `CreateClickResp = {Key, Elements, Image, Width, Height}` JSON tag 为小写，实现 `(*Handler).CreateClick` 与 `(*Handler).VerifyClick` 两个 gin handler，把 `infra/captcha` 的 sentinel error 映射到 `captcha.invalid_input/not_found/expired/mismatch/internal` 错误码与对应 HTTP 状态，并验证 `go build ./...` 通过
- [x] 2.2 新建 `internal/service/captcha/service.go` 薄包装 `infra/captcha.Manager`（直接转发 + 错误归类），并验证 `go vet ./...` 无新增告警
- [x] 2.3 新建 `internal/router/common/common.go` 用 `registry.Register` 自注册 `GET /common/captcha/create` 与 `POST /common/captcha/verify`，并在 `internal/router/index.go` 加空白导入 `_ "ai-go-mall/internal/router/common"`，并验证 `go run ./cmd/serve` 启动日志里出现 `POST /common/captcha/verify` 与 `GET /common/captcha/create` 两条路由
- [x] 2.4 单元测试 handler：用 `httptest.NewRecorder` + 内存 mock `infra/captcha.Manager`（或仅 mock service 接口）覆盖 401 mismatch / 404 not_found / 410 expired / 400 invalid_input 四种分支，并验证 `go test ./internal/handler/captcha/ -v` 全绿

## 3. 登录链路集成

- [x] 3.1 在 `internal/handler/admin/admin.go` 的 `LoginRequest` 加 `CaptchaKey string \`json:"captcha_key" binding:"required"\`` 字段，并验证单元测试（admin service_login_test.go）相应调整后通过 —— `go test ./internal/service/admin/ -v`
- [x] 3.2 在 `internal/service/admin/admin.go` 增加 `ErrInvalidCaptcha` 哨兵错误；改造 `Service.Login` 签名为 `Login(c, username, password, captchaKey, points, remember)`；顺序门控：先 `m.captcha.VerifyClick(..., deleteOnSuccess=true)` → 失败返回 `ErrInvalidCaptcha`；再做密码/状态校验，并验证单元测试覆盖"captcha 错误不进入密码校验分支"（用 mock captcha 注入失败验证密码分支未被调用）
- [x] 3.3 在 `internal/router/admin/admin.go` 装配链路里加 `captchaService` 依赖注入（参考 `adminTokenInstance` 同样的 lazy-init 模式），并验证 `go build ./...` 通过且 `go run ./cmd/serve` 启动无 panic

## 4. 前端 clickCaptcha 组件改造

- [x] 4.1 把 `web/src/components/clickCaptcha/index.vue` 的 `state.captcha` 字段从 `{id, text, base64, width, height}` 改为 `{key, elements: string[], image, width, height}`，并把 `text` 渲染循环改为对 `elements` 数组渲染，并验证 Vite HMR 不报错 + DOM 中可见"按顺序点击：A、B"提示
- [x] 4.2 把 `onRecord` 累计的 `state.xy: string[]` 改为 `state.points: {x:number, y:number}[]`，提交体从 `{uuid, captchaInfo, unset}` 改为 `{key: state.captcha.key, points: state.points, w: state.captcha.width, h: state.captcha.height}`，并验证打开 devtools 看 Network 请求 payload 形态正确
- [x] 4.3 把 `callback` 签名从 `(captchaInfo: string) => void` 改为 `(captchaKey: string, points: {x:number;y:number}[]) => void`，回调入参透传 `state.captcha.key` + `state.points`，并验证 `vue-tsc --noEmit` 通过
- [x] 4.4 修复 `offsetX/offsetY` 与图片原始 350x200 坐标不一致：在 `onRecord` 里改用 `(event.clientX - rect.left) * (img.naturalWidth / rect.width)` 归一化到原始像素；并验证 `<img>` CSS 通过 v-bind 引用 `state.captcha.width/height`（= 350/200），与 naturalWidth 匹配

## 5. 前端 login.vue 接入

- [x] 5.1 把 `web/src/api/admin/index.ts` 的 `login()` 请求体类型 `LoginRequest` 加 `captcha_key: string` 与 `points: {x:number;y:number}[]`，并验证 `vue-tsc --noEmit` 通过（仅 pre-existing random.ts 报错，与本变更无关）
- [x] 5.2 在 `login.vue` 改造 `onSubmit`：先调 `clickCaptcha(shortUuid(), (captchaKey, points) => { submitLogin(captchaKey, points) })`；弹窗未关闭前不应触发 login；并验证手动从 console 调用时序符合预期
- [x] 5.3 登录请求成功路径：把 `captchaKey` 传给 `login()`；失败路径：errorMsg 显示后端返回的 `message` 而非 axios 默认 "Request failed with status code XXX"（顺手把现有 401 文案问题也修了）

## 6. 集成验证

- [x] 6.1 Playwright e2e：开启后端 + 前端 dev，访问 `/#/admin/login`，弹窗出现 → 在 350x200 图片上点击 elements 顺序对应的位置 → 看到成功提示 → 自动关弹窗 → 登录请求带 `captchaKey` → 跳 `/admin/loading`，并验证网络面板 3 次请求（1 create + 1 verify 预检 + 1 login）顺序符合预期
- [x] 6.2 Playwright e2e：答错 3 次路径 —— 故意点错位置触发 3 次 verify 失败，验证第 3 次失败响应后前端自动重新 `getClickCaptcha`（即 create 端点被再次调用），并验证 `curl GET /common/captcha/verify/{原key}` 返回 404 `captcha.not_found`
- [x] 6.3 Playwright e2e：验证错误码透出 —— 故意答错 1 次后立即关弹窗再调登录（绕过前端预检），验证 login 返回 `login.invalid_captcha` 且**未**走到密码分支（mock 注入可观测：admin_test.go 的 TestLogin_CaptchaFailed_ShortCircuits 断言 repo.GetByUsername 未被调用）
- [x] 6.4 回归测试：`go test -count=1 ./...` 全绿 + `pnpm typecheck` 仅剩 pre-existing random.ts 报错（与本变更无关）+ 既有登录 e2e（root / Passw0rd!）仍然通过