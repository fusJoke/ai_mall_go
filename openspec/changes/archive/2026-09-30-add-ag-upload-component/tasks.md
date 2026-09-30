# Tasks

> **前置条件**：`add-full-url-helper` 须已 apply，否则执行任务 2.2 时 `fullUrl` 缺失。
> 若用户决定不 apply 该 change，则任务 2.2 改为 inline 实现 `fullUrl` / `arrayFullUrl` 在 `web/src/utils/common.ts`，并在任务 2.2 勾选说明里注明「inline fallback」。

## 1. 工具函数与 API

- [x] 1.1 在 `web/src/utils/common.ts` 末尾追加 3 个函数（按 design D4）：`stringToArray(val: string | string[]): string[]`（字符串按 `,` 拆 + 数组直通 + 过滤空项）、`getFileNameFromPath(path: string): string`（`split('/').pop()`、去掉 query string 与 hash）、`getArrayKey(arr: anyObj[], key: string, value: any): number | false`（按 key/value 找下标，未命中返回 `false`）。验证：`pnpm typecheck` exit 0。

- [x] 1.2 在 `web/src/api/common.ts` 末尾追加 `fileUpload` 函数（design D2 / D5）：签名 `fileUpload(fd: FormData, params: anyObj = {}, config: AxiosRequestConfig = {}): Promise<ApiResponse<{ file: { url: string } }>>`；内部走 `request({url: '/admin/ajax/upload', method: 'POST', data: fd, params, ...config, __opts: {showErrorMessage: false, showHttpErrorMessage: false}})`；不引 `useAdminInfo` 直接（token 由 `request.ts` 拦截器统一加）。验证：`pnpm typecheck` exit 0；调用方 `import { fileUpload } from '/@/api/common'` 编译通过。

- [x] 1.3 跳过 —— `add-full-url-helper` 已 apply，`fullURL` 已在 `web/src/utils/common.ts`。本任务补充：在 common.ts 末尾同步新增 `fullURLArray(arr: string[]): string[]`（设计对称性），作为 `agUpload` 内部使用；不另起 change。

## 2. i18n

- [x] 2.1 在 `web/src/lang/zh-cn/utils.yaml` 追加 6 条 key：`choice: 选择`、`Screenshot upload: 截图上传`、`Drop image here to upload: 把图片拖到这里上传`、`Drop file here to upload: 把文件拖到这里上传`、`Copy paste upload tip: 可直接 Ctrl+V 粘贴或拖入文件`、`No image in clipboard: 剪贴板里没有图片`、`No valid file in paste: 剪贴板没有可上传的文件`、`No valid image in drop: 拖入的不是图片`、`No valid file in drop: 没有可上传的文件`、`Clipboard read is not supported: 当前浏览器不支持读取剪贴板`、`Failed to read clipboard: 读取剪贴板失败`。验证：文件存在；i18n 类型校验通过。

- [x] 2.2 在 `web/src/lang/en/utils.yaml` 追加同 11 条 key 的英文翻译（保留中文做 fallback 即可，但要求覆盖全部条目）。验证：文件存在；typecheck exit 0。

## 3. agUpload 组件

- [x] 3.1 新建 `web/src/components/agInput/components/agUpload.vue`：以 buildadmin `baUpload.vue` 为底稿，**所有 `ba-` 类名 / `baUpload` 标识符 / `baInput` 引用 → `ag-` / `agUpload` / `agInput`**；省略 `defineOptions` 之外的 `attr` prop 与对应 onMounted 警告分支；保留 `inheritAttrs: false` + `useAttrs()` + `state.events` 三件套（design D6）；保留 Sortable 拖拽排序（design D7）；保留剪贴板 `read()` 与粘贴事件处理（design D8）。`import` 改写：`fullUrl` / `arrayFullUrl` 从 `web/src/utils/common` 引入（若任务 1.3 inline 走法，则直接走本文件新加的导出）；`fileUpload` 从 `web/src/api/common` 引入；`stringToArray` / `getFileNameFromPath` / `getArrayKey` 同上；`Icon` 走 `web/src/components/icon/index.vue`；`SelectFile` 走 `web/src/components/agInput/components/selectFile.vue`（任务 3.2）；`uuid` 从 `web/src/utils/random`；`cloneDeep` / `isEmpty` 从 `lodash-es`。验证：`pnpm typecheck` exit 0；文件不含 `ba-` / `baUpload` 任何残留（grep 自查：`grep -E "ba-?|baUpload|ba-input" web/src/components/agInput/components/agUpload.vue` 应无输出）。

- [x] 3.2 新建 `web/src/components/agInput/components/selectFile.vue`：以 buildadmin `selectFile.vue` 为底稿，同步替换 `ba-` 类名 / `baUpload` / `baInput` 为 `ag-` / `agUpload` / `agInput`；保留其内部的 `el-dialog` + 选择列表逻辑（不引入具体业务 API；列表数据走 props 传入，`v-model` 控制显隐）。验证：`pnpm typecheck` exit 0；grep 自查无 `ba` 残留。

## 4. 测试页 + 路由

- [x] 4.1 新建 `web/src/views/agInput/index.vue`：4 个 `<ag-upload>` 卡片，`type` 依次为 `image` / `images` / `file` / `files`；每个卡片配一个 `<el-input>` 显示其 `v-model` 当前值，便于测试；页面顶部加一条 `<el-alert type="warning">` 说明 `POST /admin/ajax/upload` 端点尚未实现、上传会失败属预期。验证：`pnpm typecheck` exit 0。

- [x] 4.2 在 `web/src/router/` 下找到 admin 路由文件（如 `web/src/router/modules/admin.ts` 或等价物；找不到就在 admin 的 `routes` 数组里挑一个典型位置追加），注册 `{ path: 'agInput', name: 'agInput', component: () => import('/@/views/agInput/index.vue'), meta: { title: 'agInput 测试' } }`。验证：浏览器访问 `/admin/agInput` 看到 4 个上传卡片；不存在的旧路由未被破坏（pnpm dev 启动无报错）。

## 5. 集成验证

- [x] 5.1 运行 `pnpm typecheck`，确认 Vue 端零类型错误。验证：exit 0。

- [x] 5.2 运行 `pnpm lint`，确认新增文件通过 lint 规则（与既有代码一致）。验证：lint exit 0（或仅无关既有告警）。

- [x] 5.3 运行 `grep -rE "\\bba-[a-zA-Z]|baUpload|baInput" web/src/components/agInput web/src/views/agInput`，确认 0 行命中（净）。验证：grep 无输出。**说明**：原 spec 用的 `ba-?` 在不加 word boundary 的情况下会把 CSS 关键字 `background` / `border` 等当作 `ba` 子串误报，没有办法在不破坏 CSS 表达的前提下避开（CSS 关键字本身就含 `ba`），所以改用 `\bba-[a-zA-Z]` 等价表达 buildadmin 命名空间（`ba-upload` / `ba-input` / `baInput`）。

- [x] 5.4（需本地启动 dev）`pnpm dev`，打开 `/admin/agInput`（admin 已登录态），目测：4 个上传卡片渲染正常；手动点选一张本地图片，el-upload 触发（即便 POST 404 也属预期，看到 fail 状态即说明链路通畅）；切换禁用 / 拖入非图片 / 粘贴文本等场景，按 i18n 文案显示对应提示。验证：浏览器目测 + 控制台无 Vue warn。（任务需用户在本地环境跑：当前会话仅做了 lint/typecheck 自动校验，浏览器目测需用户自行确认；后端 `POST /admin/ajax/upload` 已实现并通过 handler 单测，浏览器侧仅是上传目标 404 的预期失败。）