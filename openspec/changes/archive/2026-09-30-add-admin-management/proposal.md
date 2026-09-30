# Proposal

## Why

后台目前已经具备登录（`admin-login`）、登录后初始化（`admin-init`）以及分组 / 规则权限底座（`admin-permission`），但**对管理员账号本身的增删改查没有入口**：`Admin` 模型已经存在，`*handler.BaseHandler[model.Admin]` 也已嵌入到 `*Handler`，但 5 条通用 CRUD 路由没挂出去，前端也缺少对应的管理页。没有这条路，新员工入职、离职交接、临时账号开通 / 关闭都只能直接改库，运维成本与风险都很高。

## What Changes

- **挂出 5 条通用 CRUD 路由** 到 `POST/GET /admin/admin/{create,list,edit,delete}`，走已有的 `BaseHandler[model.Admin].RegisterRoutes`，复用 `service.CRUDService[model.Admin]`。
- **新增 4 个 admin 专属 service 方法 + handler 接口**：
  - `ChangePassword(id, newPassword)` —— 仅修改密码，service 层用 bcrypt 哈希后写入；
  - `ToggleStatus(id)` —— 切换 `Status` 字段（启用 ↔ 禁用），禁用时吊销该账号所有未过期 token；
  - `Unlock(id)` —— 把 `LoginFailure` 清零并恢复 `Status = 1`；
  - `BatchDelete(ids)` —— 一次删除多个账号，禁止把自己（当前登录）一起删。
- **service 层在 Create / Update 时自动哈希密码**：收到非空明文 `Password` 字段时调用 bcrypt；空字符串视为"不修改密码"（用于 EditPost 部分更新）。
- **前端管理页** `web/src/views/admin/manager/index.vue`：列表（搜索 + 分页）+ 新建 / 编辑弹窗（表单 + 头像上传走 agUpload）+ 重置密码 / 启停 / 解锁 / 批量删除按钮。
- **新增路由 `/admin/manager`（静态 child route，注册到 `adminBase.ts`）**，并新增对应 i18n key 与菜单规则种子数据（`admin_rule`）。

### 非目标

- **不做 admin 自助改密**：本期只支持「管理员替其他管理员」改密；admin 改自己的密码走单独的 change-password capability（后续 change）。
- **不做 2FA / OTP / OAuth**：本期只覆盖账号生命周期管理。
- **不动 admin-init 行为**：登录后的初始化响应只读 `Admin`，不影响写路径。

## Capabilities

### New Capabilities
- `admin-management`: 后台管理员账号的增删改查 + 专属操作（改密 / 启停 / 解锁 / 批量删除）。前端在 `/admin/manager` 页面提供完整操作能力。

### Modified Capabilities
<!-- 现有 capability 无需求级变化。admin-login / admin-init / admin-permission 都保持现状。 -->

## Impact

- **后端**：
  - `internal/handler/admin/admin.go` —— 暴露 `ChangePassword` / `ToggleStatus` / `Unlock` / `BatchDelete` 4 个新 handler 方法；
  - `internal/service/admin/admin.go` —— 新增上述 4 个 service 方法 + 在 `Create` / `Update` 路径上挂 bcrypt；
  - `internal/repository/admin/admin_repository.go` —— 视实现细节可能新增 `DeleteBatch(ids)` / `UpdateStatus` 等专用方法；
  - `internal/router/admin/admin.go` —— 注册 `BaseHandler.RegisterRoutes` + 4 个专属路由，全部挂 `AdminAuth` 中间件。
- **前端**：
  - `web/src/views/admin/manager/index.vue` —— 新增；
  - `web/src/api/admin.ts` —— 新增对应 API；
  - `web/src/lang/{zh-cn,en}/{admin,utils}.yaml` —— 新增 12 条 i18n key；
  - `web/src/router/static/adminBase.ts` —— 静态子路由 `/admin/manager` 注册；
  - `cmd/migrate/migrations/` —— 新增 `admin_rule` 菜单规则种子数据迁移文件（若选 SQL 路径）或在 init.go 内存种子。
- **数据库**：
  - 不新增表；`Admin` 模型已含所有字段，复用现有 `admins` 表。
- **权限**：新增的「账号管理」菜单规则需要在 `admin_rule` 表里有种子记录（仅超管可访问，按 `admin-permission` 既有约定）。
