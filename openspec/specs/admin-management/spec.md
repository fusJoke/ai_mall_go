# Admin Management Specification

## Purpose

Lets administrators manage backend admin accounts through a web UI: list, search, create, edit, delete, reset password, enable / disable, unlock, and batch-delete — with bcrypt-hashed passwords, status / login-failure invariants, and self-protection against deleting the currently logged-in account.

## Requirements

### Requirement: Admin can list and search admin accounts

The system MUST expose `GET /admin/admin/list` that returns a paginated list of admin accounts. The response SHALL include `items`, `total`, `page`, `page_size`. The endpoint MUST accept `?page=`, `?page_size=`, and at least one of `?username=` or `?nickname=` as optional fuzzy-match filters. The endpoint MUST NOT include the `password` field in any list item.

#### Scenario: Default pagination
- **WHEN** an authenticated admin calls `GET /admin/admin/list` without query params
- **THEN** the response is `200` with `{items: [...], total, page: 1, page_size: 20}`

#### Scenario: Filtered by username
- **WHEN** an authenticated admin calls `GET /admin/admin/list?username=ali`
- **THEN** every returned item's username contains the substring `ali` (case-insensitive)

#### Scenario: Page size cap
- **WHEN** an authenticated admin calls `GET /admin/admin/list?page_size=9999`
- **THEN** the server caps `page_size` at 200 and returns at most 200 items

#### Scenario: Password field never returned
- **WHEN** any admin row is returned through the list endpoint
- **THEN** the `password` field is absent from every item

### Requirement: Admin can create a new admin account

The system MUST expose `POST /admin/admin/create` accepting a JSON body with at least `username` and `password`. The system MUST reject requests where `username` is empty, already exists, or shorter than the configured minimum (default 3 chars); or where `password` is shorter than the configured minimum (default 8 chars). The system MUST hash the password with bcrypt before persisting and MUST store the bcrypt hash, never the plaintext.

#### Scenario: Successful creation
- **WHEN** an authenticated admin posts `{username: "alice", password: "S3cretPwd!", nickname: "Alice"}` to `/admin/admin/create`
- **THEN** the response is `200` with the created admin (including the new id, but `password` absent)
- **AND** the row in `admins` stores a bcrypt hash whose prefix is `$2a$` / `$2b$` / `$2y$`, not the plaintext

#### Scenario: Username already exists
- **WHEN** an authenticated admin posts a username that already exists in `admins`
- **THEN** the response is `4xx` with code `admin.create.duplicate_username` and the row is not inserted

#### Scenario: Password too short
- **WHEN** an authenticated admin posts a password shorter than the configured minimum
- **THEN** the response is `4xx` with code `admin.create.password_too_short` and the row is not inserted

### Requirement: Admin can edit an existing admin account

The system MUST expose `POST /admin/admin/edit` accepting a JSON body that contains at least `id`. The system MUST reject requests where `id` is missing or zero. When the body contains a non-empty `password` field, the system MUST hash it with bcrypt before persisting; when the field is absent or empty, the system MUST NOT modify the stored password. The endpoint MUST NOT allow changing `username` to a value already taken by another row.

#### Scenario: Edit nickname only
- **WHEN** an authenticated admin posts `{id: 7, nickname: "Alice2"}` to `/admin/admin/edit`
- **THEN** the response is `200` and the stored `password` is unchanged

#### Scenario: Edit with new password
- **WHEN** an authenticated admin posts `{id: 7, password: "NewPwd!2026"}` to `/admin/admin/edit`
- **THEN** the response is `200` and the stored password is now the bcrypt hash of `NewPwd!2026`

#### Scenario: Edit zero id
- **WHEN** an authenticated admin posts `{nickname: "x"}` (no `id`) to `/admin/admin/edit`
- **THEN** the response is `400`

### Requirement: Admin can delete an admin account

The system MUST expose `POST /admin/admin/delete` accepting the target id via query string `?id=` or form field `id=`. The system MUST reject deletion when the target id equals the currently authenticated admin's id and MUST respond with `403 admin.delete.self_protection`. Otherwise the system MUST soft-delete the row (preserving audit trail) and respond with `200 {ok: true}`.

#### Scenario: Delete another admin
- **WHEN** an authenticated admin posts `POST /admin/admin/delete?id=42` and the current admin's id is not 42
- **THEN** the response is `200 {ok: true}` and the row's `deleted_at` is set

#### Scenario: Delete self
- **WHEN** an authenticated admin posts `POST /admin/admin/delete?id=<self>` where `<self>` is the current admin's id
- **THEN** the response is `403 admin.delete.self_protection` and the row is not modified

### Requirement: Admin can change another admin's password

The system MUST expose `POST /admin/admin/change-password` accepting `{id, new_password}` in the JSON body. The system MUST validate `new_password` against the same minimum-length rule as create (default 8 chars). The system MUST hash `new_password` with bcrypt and overwrite the stored password. After successful change, the system MUST revoke all active tokens for that admin so previously issued sessions can no longer access protected endpoints.

#### Scenario: Successful password change
- **WHEN** an authenticated admin posts `{id: 7, new_password: "BrandNew!2026"}` to `/admin/admin/change-password`
- **THEN** the response is `200` and the stored password for id=7 is the bcrypt hash of `BrandNew!2026`
- **AND** any existing token rows where `user_id = 7` AND `type = 'admin'` AND not soft-deleted are soft-deleted

#### Scenario: Password too short
- **WHEN** an authenticated admin posts `{id: 7, new_password: "short"}` to `/admin/admin/change-password`
- **THEN** the response is `4xx` with code `admin.change_password.password_too_short` and the stored password is unchanged

### Requirement: Admin can toggle an admin account's status

The system MUST expose `POST /admin/admin/toggle-status` accepting `{id}` in the JSON body. When the current value of `Status` is `1`, the system MUST set it to `0` and revoke all active admin tokens for that id. When the current value is `0`, the system MUST set it to `1`. The system MUST refuse to disable the currently authenticated admin and respond with `403 admin.toggle_status.self_protection`.

#### Scenario: Disable another admin
- **WHEN** an authenticated admin posts `{id: 42}` and admin 42 currently has `Status = 1`
- **THEN** the response is `200` and admin 42's `Status` is now `0`
- **AND** any existing token rows where `user_id = 42` AND `type = 'admin'` AND not soft-deleted are soft-deleted

#### Scenario: Enable a disabled admin
- **WHEN** an authenticated admin posts `{id: 42}` and admin 42 currently has `Status = 0`
- **THEN** the response is `200` and admin 42's `Status` is now `1`

#### Scenario: Disable self
- **WHEN** an authenticated admin posts `{id: <self>}` to `/admin/admin/toggle-status`
- **THEN** the response is `403 admin.toggle_status.self_protection` and `Status` is unchanged

### Requirement: Admin can unlock a locked admin account

The system MUST expose `POST /admin/admin/unlock` accepting `{id}` in the JSON body. The system MUST reset `LoginFailure` to `0` and set `Status = 1`. The endpoint MUST NOT modify `Password`.

#### Scenario: Unlock a locked account
- **WHEN** an authenticated admin posts `{id: 42}` and admin 42 currently has `LoginFailure = 5` AND `Status = 0`
- **THEN** the response is `200` and admin 42's `LoginFailure` is now `0` AND `Status` is now `1` AND `Password` is unchanged

### Requirement: Admin can batch delete admin accounts

The system MUST expose `POST /admin/admin/batch-delete` accepting `{ids: [int]}` in the JSON body. The system MUST remove the self id from `ids` before processing and MUST respond with `{deleted: n, skipped_self: 1}` so the caller can observe that self-protection kicked in. Each remaining id is soft-deleted. Empty `ids` (or `ids` containing only self) MUST respond with `200` and `deleted: 0`.

#### Scenario: Batch delete mixed selection
- **WHEN** an authenticated admin with id=1 posts `{ids: [1, 2, 3]}` to `/admin/admin/batch-delete`
- **THEN** the response is `200 {deleted: 2, skipped_self: 1}` and admins 2 and 3 are soft-deleted while admin 1 is not

#### Scenario: Batch delete empty
- **WHEN** an authenticated admin posts `{ids: []}` to `/admin/admin/batch-delete`
- **THEN** the response is `200 {deleted: 0, skipped_self: 0}`

### Requirement: Passwords are never exposed through any endpoint

The system MUST strip the `password` field from every JSON response that returns an admin record (list, create, edit, toggle-status, unlock, change-password). A direct read by id MUST also omit `password`. The bcrypt hash SHALL NEVER appear in any HTTP response body.

#### Scenario: List strips password
- **WHEN** an authenticated admin calls `GET /admin/admin/list`
- **THEN** no item in `items` contains a `password` field

#### Scenario: Create strips password
- **WHEN** an authenticated admin creates a new admin successfully
- **THEN** the response body has no `password` field

### Requirement: Management UI is available at /admin/manager

The system MUST expose a Vue route `/admin/manager` under the admin layout that renders a single page (`web/src/views/admin/manager/index.vue`) containing: (1) a paginated table listing admin accounts with search input; (2) a "create" button that opens a dialog with form fields `username`, `nickname`, `email`, `mobile`, `avatar`, `password`, `bio`, `status`; (3) per-row actions for edit, change-password, toggle-status, unlock, delete; (4) batch delete via selected rows. The page MUST call the JSON endpoints above. The page MUST use the existing i18n keys with English fallback.

#### Scenario: Visiting /admin/manager
- **WHEN** a logged-in admin opens `/admin/manager`
- **THEN** the page renders the table, search input, and create button
- **AND** the table is populated by calling `GET /admin/admin/list` on mount

#### Scenario: Create via dialog
- **WHEN** the user fills in the dialog and clicks "Save"
- **THEN** the page calls `POST /admin/admin/create` and on success refreshes the table

#### Scenario: Self-protected delete in UI
- **WHEN** the current admin clicks the delete button on their own row
- **THEN** the page disables the delete control for that row (the backend self-protection is the source of truth; the UI is best-effort UX)