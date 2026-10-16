# Design

## Context

GUI 登录验证（2026-10-01）实测发现：`Login` 两条路径把**从 DB 读出的完整模型**（含 bcrypt 哈希形态的 `Password`）交给 `s.Update`，而 `Update` 的「Password 非空即 bcrypt」语义把哈希再哈希一次。逐次比对 DB 哈希确证：登录成功一次后哈希变为 `bcrypt(bcrypt(pw))`，同密码再登录 401。

## Goals / Non-Goals

- Goals：登录链路（成功 / 失败）对 Password 列零改写；沿用 repo 既有的定向列更新风格；回归测试锁死该行为。
- Non-Goals：不改 `Update`/`hashPasswordIfNeeded` 语义（管理页部分更新依赖）；不修复存量已腐化数据；不做并发计数原子化。

## Decisions

### D1. 定向列更新，而非「哈希格式探测」防御

两个候选方案：

| 方案 | 做法 | 弃选 / 采纳理由 |
| --- | --- | --- |
| A. 定向列更新（采纳） | repo 新增 `UpdateLoginFailure` / `UpdateLoginSuccess`，Login 不再走 `s.Update` | 写路径本身不携带 Password，**结构上不可能**再腐化；与 repo 既有 `UpdateStatus` / `UpdatePassword` / `ResetLoginFailure` 风格一致（`admin.go` 注释已写明"专用列更新省一次 SELECT + 锁"）；省去整行 UPDATE |
| B. 哈希探测防御 | `hashPasswordIfNeeded` 检测到 60 字符 `$2a$` 前缀就跳过 | 掩盖调用方误用；对 argon2 等未来算法 / 前缀规则变化脆弱；合法"改密为字面量 `$2a$...`"被静默吞掉；仍走整行 UPDATE |

采纳 A。`baseService.Update` 的「非空即哈希」语义保持不变 —— 它对管理页 DTO 部分更新（省略 password 字段 = 不改密）是正确设计；缺陷本质是调用方误用，修调用方而非弱化校验。

### D2. 失败分支接口签名：`(id, failure, locked bool)`，锁定列合并进同一方法

密码错误时 service 计算 `failure = adm.LoginFailure + 1` 与 `locked = failure >= MaxLoginFailure`（阈值判定留在 service，repo 不掺业务）。锁定时需同时写 `login_failure` 与 `status=0`：

- 若拆成两次调用（`UpdateLoginFailure` + 既有 `UpdateStatus`）：锁定路径两条 UPDATE 非原子，且要多一次往返；
- 合并为一个方法、`locked=true` 时 map 中追加 `"status": 0`：单条 UPDATE，语义为「失败计数 + 触锁」，与该分支业务动作一一对应。

`locked=false` 时 map 仅含 `login_failure`，status 列不动（GORM map Updates 只写 map 内列）。

### D3. 成功分支接口签名：`(id, ip, at)`，三列一次写清

`login_failure=0`、`last_login_at=at`、`last_login_ip=ip` 同属"登录成功"这一个业务事件，合并为 `UpdateLoginSuccess` 单条 UPDATE；`at` 由 service 传（token `ExpiresAt` 也要用同一时刻，保持两处时间一致）。

### D4. 回归断言层面选在 service mock，而非只测 repo SQL

repo sqlmock 只能证明"新方法只写指定列"；腐化缺陷的根因是**调用方**。service 层用例必须断言：密码错误 / 登录成功两条路径上 `repo.Update`（哈希入口）调用次数为 0，新方法调用次数为 1。两层各自锁死一半。

## Risks / Trade-offs

- 接口扩展 → 所有 mock 实现需同步补方法（测试内 mock + 未来实现方），一次性成本。
- 失败计数仍为"读-改-写"非原子（与现状一致）：并发下可能少计 1 次，方向幂等（最终朝锁定收敛），本次不扩大处理面（见 proposal 非目标）。
- 存量已腐化账号（如 `admin` id=1）不因本 change 自愈：运维上走管理页改密 / `cmd/seed` 重置。

## Migration Plan

1. repo 加方法（纯增量）→ 2. service 切换调用点 → 3. 测试迁移 + 回归用例 → 4. `go test` 全绿 → 5. GUI 回归（登录→登出→再登录成功）。无 DB 迁移、无停机面。

## Open Questions

无。
