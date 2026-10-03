# admin-log Specification

## Purpose

管理员日志查看页：后台运营检索「谁在什么时间从哪个 IP 对哪个路径做了什么操作」。页面纯前端实现，数据层当前为示例数据（API 契约与服务端 `/admin/admin-log/*` 未来实现对齐），服务端接入后仅替换 API 函数体。

## ADDED Requirements

### Requirement: 日志列表与检索

管理员日志页 SHALL 提供分页列表（id/管理员/操作标题/请求路径/IP/操作时间列）与关键字搜索（模糊匹配 username/title/url/ip，重置页码后过滤）。数据 SHALL 经 `adminLogList` API 获取（分页 + keyword），当前为示例数据实现。

#### Scenario: 关键字搜索

- **WHEN** 用户输入 "admin" 并点击搜索
- **THEN** 列表仅显示 username 含 "admin" 的记录，分页总数同步更新

#### Scenario: 分页切换

- **WHEN** 用户切换每页条数为 50
- **THEN** 列表按新页大小重新加载

### Requirement: 日志详情

每行 SHALL 提供「详情」操作：弹窗以 descriptions 形式展示全字段，含 admin_id 与 User-Agent 完整文本。

#### Scenario: 查看详情

- **WHEN** 用户点击某行「详情」
- **THEN** 弹窗展示该条记录的全部字段（含 User-Agent）

### Requirement: 日志删除

页面 SHALL 支持单条删除与多选批量删除，均需二次确认；删除后 SHALL 刷新列表。当前删除为内存级示例数据操作（刷新后恢复），API 形态与服务端对齐。

#### Scenario: 批量删除

- **WHEN** 用户勾选 3 条并点击「批量删除」、确认
- **THEN** 提示删除数量，列表刷新且这 3 条不再出现
