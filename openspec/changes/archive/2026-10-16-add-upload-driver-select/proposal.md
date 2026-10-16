# Proposal

## Why

`agUpload` 组件当前只会把附件传到服务端本地存储（`force_local` 参数）。真实业务需要多驱动上传（本地 / 阿里云 OSS / 腾讯云 COS / 七牛云 Kodo）：前端要能声明「这次上传走哪个驱动」并让调用方在 UI 上选择验证。服务端驱动的实际生效逻辑不在本次范围——上传请求增加 `driver` 参数即可，未配置驱动时服务端回落本地。

## What Changes

- **扩展 `web/src/components/agInput/components/agUpload.vue`**：新增 `driver?: string` prop（默认 `'local'`），随 `fileUpload` 请求额外参数发送（与既有 `force_local` 并列）。
- **新增 `web/src/mock/uploadDrivers.ts`**：上传驱动示例数据 —— 4 个驱动（local/aliyun/tencent/qiniu）的 value/name/description/configured/isDefault。
- **改造 `web/src/views/agInput/index.vue`**：顶部新增「上传驱动（示例数据）」选择卡片区（4 张驱动卡片 + 当前驱动提示），四种上传形态（image/images/file/files）联动所选 driver。

### 非目标

- **不实现服务端驱动**：OSS/COS/Kodo 的服务端直传/回落逻辑不在本次范围；`driver` 参数当前由服务端忽略（回落本地）。
- **不做驱动配置管理页**：驱动的 AccessKey/Bucket 配置界面（buildadmin 的常规管理-上传配置）留待后续 change。
- **不改动 agUpload 既有交互**：进度/拖拽/粘贴/排序/选择器等行为不变。

## Capabilities

### New Capabilities

无。

### Modified Capabilities

- `ag-upload`: `agUpload` 组件新增 `driver` prop 随上传请求发送；`/agInput` 演示页增加驱动选择区。

## Impact

- **代码**：修改 2 个前端文件 + 新增 1 个示例数据文件；无后端改动。
- **规格**：修改 `ag-upload` capability 的 2 条 Requirement。
- **验证**：`pnpm typecheck`；GUI 黑盒——/agInput 切换驱动后上传，请求参数带 `driver=<所选值>`；不传 prop 时默认 `local` 行为不变。
