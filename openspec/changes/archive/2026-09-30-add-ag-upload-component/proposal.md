# Proposal

## Why

后台侧每个表单（管理员头像、轮播图、商品图、富文本附件）都需要一个上传组件，但当前项目没有可复用的 Vue 3 上传封装。每处业务都重复写 el-upload 配置、手写拖拽 / 粘贴 / 进度回调，导致：

1. **样式 / 交互不一致** —— 不同页面的拖拽遮罩、粘贴提示、限制提示文案各做各的；
2. **后端协议散落** —— 有的走 multipart form，有的发 base64，前端无法集中控制；
3. **可访问性 / 国际化缺失** —— 没有键盘焦点、没有 i18n key。

参考 buildadmin v2 的 `baInput/components/baUpload.vue` 已覆盖 90% 需求（picture-card 卡片拖拽上传、文件流式上传、剪贴板粘贴、拖拽排序、上传进度）。本次把它按项目命名规范（`ba` → `ag`）移植过来，并把缺失的辅助函数 / API / i18n 配套补齐，让业务侧一行 `<ag-upload v-model="avatar" type="image" />` 即可使用。

## What Changes

- 在 `web/src/components/agInput/components/agUpload.vue` 新增前端上传组件（基于 el-upload + Sortable + 自定义拖拽 / 粘贴）。`ba-` 类名前缀、`baUpload` / `state` / 事件名 → 改为 `ag-` / `agUpload`（例：`ba-upload-wrapper` → `ag-upload-wrapper`，`ba-upload-trigger` → `ag-upload-trigger`）。
- 在 `web/src/components/agInput/components/selectFile.vue` 同步移植附件选择弹窗组件（agUpload 默认要嵌入它）。类名同步改 `ag-` 前缀。
- 在 `web/src/api/common.ts` 新增 `fileUpload(fd, params, onUploadProgress?)` 函数：调用现有 `request`（默认导出 axios 实例）POST 到 `/admin/ajax/upload`，携带 `Authorization` token，复用项目统一信封解包与错误提示。返回 `Promise<ApiResponse<{ file: { url: string, ... } }>>`。
- 在 `web/src/utils/common.ts` 新增 5 个辅助函数（buildadmin 同名同形）：`fullUrl`、`arrayFullUrl`、`getFileNameFromPath`、`getArrayKey`、`stringToArray`。**注意**：`fullUrl` 与 `add-full-url-helper` change 规划的 `fullURL` 是同一函数（后者只是大小写差异 + 单测覆盖）。本 change 落地要求 `add-full-url-helper` 已 apply；若实际落地时该 change 仍未 apply，则将 `fullUrl` / `arrayFullUrl` 作为 `add-full-url-helper` 提案的子集同步实现（写在 common.ts 即可），二选一不重复实现。
- 在 `web/src/lang/zh-cn/utils.yaml` 与 `web/src/lang/en/utils.yaml` 新增 6 条 i18n key：`choice`（选择）、`Screenshot upload`（截图上传）、`Drop image here to upload` / `Drop file here to upload`、`Copy paste upload tip`、`No image in clipboard` / `No valid file in paste` / `No valid image in drop` / `No valid file in drop`、`Clipboard read is not supported` / `Failed to read clipboard` / `No image in clipboard`。
- 在 `web/src/views/agInput/index.vue` 新增一个空白使用示例页面，路由以 `/agInput` 注册（admin layout），挂载 4 个 agUpload 卡片：单图 / 多图 / 单文件 / 多文件。

## Capabilities

### New Capabilities

- `ag-upload`: 通用图片 / 文件上传组件，支持 picture-card 卡片流、点击选择、拖拽、剪贴板粘贴、上传进度、拖拽排序、附件选择弹窗，输出 `v-model` 字符串 / 数组。

### Modified Capabilities

无（现有 capability 都不涉及上传 UI）。

## Impact

- 前端组件：
  - `web/src/components/agInput/components/agUpload.vue`（新增，约 600 行）
  - `web/src/components/agInput/components/selectFile.vue`（新增，约 200 行）
- 前端 utils：`web/src/utils/common.ts`（追加 5 个函数，约 60 行）
- 前端 API：`web/src/api/common.ts`（追加 `fileUpload` 函数，约 40 行）
- 前端 i18n：`web/src/lang/{zh-cn,en}/utils.yaml`（追加约 10 条 key）
- 前端视图与路由：`web/src/views/agInput/index.vue`（新增）、`web/src/router/...` 注册 `/agInput`（admin 区）
- 前端依赖：无新增（`sortablejs@1.15.4` / `lodash-es@4.17.x` / `element-plus@2.x` / `axios@1.x` 已在 `package.json` 中）
- 前置依赖：本 change 落地前应先 apply `add-full-url-helper`（已规划），否则 `fullUrl` 实现会在两处重复
- 后端：组件 POST `/admin/ajax/upload` 端点尚不存在；落地后该请求会 404。**后端 upload handler 是单独的 change**，本 change 不引入；用户测试时需先 stub 或在 apply 前 ack 此限制
- 既有代码：无 breaking 改动（新增文件为主，common.ts 仅追加）