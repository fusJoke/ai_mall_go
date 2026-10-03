# Design

## Context

buildadmin 的管理员日志页（`views/admin/log` + `api/adminLog`）依赖服务端 `admin_log` 表；本项目服务端尚未实现。前端先以「API 形态的示例数据实现」落地页面，函数签名与真实 API 一致，后续替换函数体即可。

## Goals / Non-Goals

- Goals：日志查看/检索/删除交互完整可用；示例数据确定性可复现（便于分页/搜索测试）；API 层与服务端契约对齐。
- Non-Goals：服务端实现、日志导出、时间范围筛选。

## Decisions

### D1. 示例数据确定性生成而非随机

`buildExampleAdminLogs()` 以固定起点时间（2026-09-28）+ 固定 3 小时步进 + 固定轮换表（13 种操作 × 3 管理员 × 5 IP × 3 UA）生成 66 条记录——同一次刷新内分页/搜索结果稳定，测试可复现；不做 Math.random 抖动。

### D2. API 文件是 mock 的宿主而非独立 mock 层开关

`api/adminLog.ts` 直接 import 示例数据并在内存内做分页/过滤/删除（splice），对外暴露与 `api/rule.ts` 同形态的 Promise 约定（code=1）。不引入 mock 开关/环境变量——服务端接入时直接把函数体换成 `request.request` 调用，调用方零改动。

### D3. 操作标题语义色内联映射

tagType(title) 按中文关键字（登录/退出 → success；新增/编辑/修改/保存/删除 → warning；其余 → info）做内联映射，不建独立字典；示例数据可控，命中即可。

## Risks / Trade-offs

- 内存删除在刷新后恢复：示例数据约定，已在 API 文件头注明；无持久化需求。
- i18n key 落在 admin.yaml（`log.*`），与 rule/manager 同文件，符合既有惯例。

## Migration Plan

纯前端新增；无 schema / 后端面。

## Open Questions

无。
