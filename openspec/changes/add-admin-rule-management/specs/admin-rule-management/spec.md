# Spec Delta

## Purpose

Lets administrators manage the `admin_rule` table — the menu / permission rule tree — through a web UI: list, search, create, edit, delete, toggle status, batch-delete, and pick a parent id for a new rule — while preserving the PID self-reference tree integrity and the existing `admin-rule` model invariants.

## ADDED Requirements

### Requirement: Admin can list and search menu rules

The system MUST expose `GET /admin/rule/list` that returns a paginated list of menu rules. The response SHALL include `items`, `total`, `page`, `page_size`. The endpoint MUST accept `?page=`, `?page_size=`, and at least one of `?title=` or `?name=` as optional fuzzy-match filters. The endpoint MUST NOT include soft-deleted rows. The response items MUST contain every field of `admin_rule` so the management page can render and edit them directly.

#### Scenario: Default pagination
- **WHEN** an authenticated admin calls `GET /admin/rule/list` without query params
- **THEN** the response is `200` with `{items: [...], total, page: 1, page_size: 20}`

#### Scenario: Filtered by title
- **WHEN** an authenticated admin calls `GET /admin/rule/list?title=用户`
- **THEN** every returned item's `title` contains the substring `用户` (case-insensitive)

#### Scenario: Page size cap
- **WHEN** an authenticated admin calls `GET /admin/rule/list?page_size=9999`
- **THEN** the server caps `page_size` at 200 and returns at most 200 items

#### Scenario: Soft-deleted rows excluded
- **WHEN** an authenticated admin calls `GET /admin/rule/list`
- **THEN** no item has `deleted_at` set

### Requirement: Admin can create a new menu rule

The system MUST expose `POST /admin/rule/create` accepting a JSON body with at least `title` and `name`. The system MUST reject requests where `title` is empty or `name` is empty. The system MUST reject requests where `type` is not one of `dir` / `menu` / `node`, where `open_type` (when present) is not one of `tab` / `link` / `iframe`, and where `extend` is not one of `none` / `add_route_only` / `add_menu_only`. The system MUST reject requests where `pid` refers to a non-existent or soft-deleted rule. On success the response MUST be `200` with the created rule including the server-assigned `id` and timestamp fields.

#### Scenario: Successful creation of a top-level menu rule
- **WHEN** an authenticated admin posts `{pid: 0, type: "menu", title: "用户管理", name: "user"}` to `/admin/rule/create`
- **THEN** the response is `200` with the created rule, including `id`, `created_at`, `updated_at`, and `status` defaulting to `1`

#### Scenario: Empty title rejected
- **WHEN** an authenticated admin posts `{title: "", name: "user"}` to `/admin/rule/create`
- **THEN** the response is `4xx` with code `rule.create.invalid_input` and the row is not inserted

#### Scenario: Unknown type rejected
- **WHEN** an authenticated admin posts `{title: "x", name: "x", type: "unknown"}` to `/admin/rule/create`
- **THEN** the response is `4xx` with code `rule.create.invalid_input` and the row is not inserted

#### Scenario: PID references non-existent rule
- **WHEN** an authenticated admin posts `{pid: 9999, title: "x", name: "x"}` to `/admin/rule/create` and rule 9999 does not exist
- **THEN** the response is `4xx` with code `rule.create.pid_not_found` and the row is not inserted

### Requirement: Admin can edit an existing menu rule

The system MUST expose `POST /admin/rule/edit` accepting a JSON body that contains at least `id`. The system MUST reject requests where `id` is missing or zero. The system MUST refuse any edit that would form a PID cycle — that is, when the new `pid` is the rule's own `id` or refers to a descendant of the rule. On success the response MUST be `200` with the updated rule including refreshed `updated_at`.

#### Scenario: Edit title only
- **WHEN** an authenticated admin posts `{id: 7, title: "用户管理2"}` to `/admin/rule/edit`
- **THEN** the response is `200` and rule 7's `title` is now `用户管理2` and `updated_at` is refreshed

#### Scenario: Edit zero id rejected
- **WHEN** an authenticated admin posts `{title: "x"}` (no `id`) to `/admin/rule/edit`
- **THEN** the response is `400`

#### Scenario: Self-parenting rejected
- **WHEN** an authenticated admin posts `{id: 7, pid: 7}` to `/admin/rule/edit`
- **THEN** the response is `4xx` with code `rule.edit.pid_cycle` and the rule is not modified

#### Scenario: Cycle through descendants rejected
- **WHEN** rule 7 has a descendant rule 12, and an authenticated admin posts `{id: 7, pid: 12}` to `/admin/rule/edit`
- **THEN** the response is `4xx` with code `rule.edit.pid_cycle` and the rule is not modified

### Requirement: Admin can delete a menu rule

The system MUST expose `POST /admin/rule/delete` accepting the target id via query string `?id=` or form field `id=`. The system MUST refuse deletion when the target rule has any non-soft-deleted child rules and MUST respond with `4xx rule.delete.has_children`. Otherwise the system MUST soft-delete the row (preserving audit trail) and respond with `200 {ok: true}`.

#### Scenario: Delete a leaf rule
- **WHEN** an authenticated admin posts `POST /admin/rule/delete?id=12` and rule 12 has no children
- **THEN** the response is `200 {ok: true}` and rule 12's `deleted_at` is set

#### Scenario: Delete a rule with children refused
- **WHEN** an authenticated admin posts `POST /admin/rule/delete?id=7` and rule 7 has at least one child rule
- **THEN** the response is `4xx` with code `rule.delete.has_children` and rule 7 is not modified

### Requirement: Admin can toggle a menu rule's status

The system MUST expose `POST /admin/rule/toggle-status` accepting `{id}` in the JSON body. When the current value of `status` is `1`, the system MUST set it to `0`. When the current value is `0`, the system MUST set it to `1`. The endpoint MUST NOT revoke any tokens (rules are not session-scoped). On success the response MUST be `200` and contain the new `status`.

#### Scenario: Disable an enabled rule
- **WHEN** an authenticated admin posts `{id: 12}` and rule 12 currently has `status = 1`
- **THEN** the response is `200` and rule 12's `status` is now `0`

#### Scenario: Enable a disabled rule
- **WHEN** an authenticated admin posts `{id: 12}` and rule 12 currently has `status = 0`
- **THEN** the response is `200` and rule 12's `status` is now `1`

### Requirement: Admin can batch delete menu rules

The system MUST expose `POST /admin/rule/batch-delete` accepting `{ids: [int]}` in the JSON body. The system MUST skip any id that does not exist, is already soft-deleted, or has children; the response MUST include `{deleted: n, skipped: m}` so the caller can observe what was skipped and why. The system MUST soft-delete each remaining id. Empty `ids` MUST respond with `200` and `deleted: 0`.

#### Scenario: Batch delete mixed selection
- **WHEN** an authenticated admin posts `{ids: [1, 2, 3]}` to `/admin/rule/batch-delete` and rule 1 has a child, rule 2 is a leaf, and rule 3 does not exist
- **THEN** the response is `200 {deleted: 1, skipped: 2}`

#### Scenario: Batch delete empty
- **WHEN** an authenticated admin posts `{ids: []}` to `/admin/rule/batch-delete`
- **THEN** the response is `200 {deleted: 0, skipped: 0}`

### Requirement: Admin can fetch all enabled menu rules for parent selection

The system MUST expose `GET /admin/rule/all` returning the full list of enabled (`status = 1`) and non-soft-deleted rules. The response MUST be a JSON array (no pagination wrapper) so the management page can populate a parent-id `<el-tree-select>` dropdown. Each item MUST contain at least `id`, `pid`, `title`, `type`, so the UI can render the tree structure.

#### Scenario: Fetching all enabled rules
- **WHEN** an authenticated admin calls `GET /admin/rule/all`
- **THEN** the response is `200` with a JSON array of every `status = 1` non-soft-deleted rule, sorted by `weigh ASC, id ASC`

#### Scenario: Disabled rules excluded
- **WHEN** an authenticated admin calls `GET /admin/rule/all` and some rules have `status = 0`
- **THEN** those disabled rules are absent from the response

### Requirement: Management UI is available at /admin/rule

The system MUST expose a Vue route `/admin/rule` under the admin layout that renders a single page (`web/src/views/admin/rule/index.vue`) containing: (1) a paginated table listing menu rules with search input on `title` and `name`; (2) a "create" button that opens a dialog with form fields `pid` (tree-select), `type`, `title`, `name`, `path`, `icon`, `open_type`, `url`, `component`, `keepalive`, `extend`, `remark`, `weigh`, `status`; (3) per-row actions for edit, toggle-status, delete; (4) batch delete via selected rows. The page MUST call the JSON endpoints above. The page MUST use the existing i18n keys with English fallback.

#### Scenario: Visiting /admin/rule
- **WHEN** a logged-in admin opens `/admin/rule`
- **THEN** the page renders the table, search input, and create button
- **AND** the table is populated by calling `GET /admin/rule/list` on mount
- **AND** the parent-id tree-select is populated by calling `GET /admin/rule/all` on mount

#### Scenario: Create via dialog
- **WHEN** the user fills in the dialog and clicks "Save"
- **THEN** the page calls `POST /admin/rule/create` and on success refreshes the table

#### Scenario: PID cycle error surfaces in UI
- **WHEN** the user attempts to edit a rule to set its parent as one of its own descendants
- **THEN** the page displays the error code `rule.edit.pid_cycle` and does not close the dialog
