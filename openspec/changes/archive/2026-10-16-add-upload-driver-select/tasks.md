# Tasks

## 1. 组件与数据层

- [x] 1.1 `web/src/components/agInput/components/agUpload.vue`：新增 `driver?: string` prop（默认 'local'），`fileUpload` 额外参数增加 `driver: props.driver`。
- [x] 1.2 新增 `web/src/mock/uploadDrivers.ts`：4 个驱动示例数据（local/aliyun/tencent/qiniu，含 configured/isDefault 标记与说明文案）。

## 2. 演示页与验证

- [x] 2.1 改造 `web/src/views/agInput/index.vue`：驱动选择卡片区（active 高亮、示例/默认 tag、当前驱动与请求参数提示）+ 四种上传形态绑定 `:driver="selectedDriver"` + v-model 当前值回显。
- [x] 2.2 `pnpm typecheck` 通过（仅既存 random.ts 遗留报错）；ESLint 零告警。
- [x] 2.3 GUI 黑盒：切换驱动后上传请求参数带 `driver=<所选值>`；默认 local 行为不变。
- [x] 2.4 `openspec validate add-upload-driver-select` 通过；勾选本文件所有任务。
