# Design

## Context

The reference is `D:\buildadmin-v2\web\src\components\baInput\components\baUpload.vue` (~800 lines, Vue 3 `<script setup>` + el-upload + Sortable + 自定义拖拽 / 粘贴) and its companion `selectFile.vue`. Project state:

- No existing upload component in `web/src/components/`.
- `web/src/api/common.ts` only has `getClickCaptcha` / `checkClickCaptcha` / `clearCache`; no `fileUpload`.
- `web/src/utils/common.ts` is utility-shaped (fullUrl / arrayFullUrl / stringToArray / getArrayKey / getFileNameFromPath do not exist).
- `package.json` already has `sortablejs@1.15.7`, `lodash-es@4.18.1`, `element-plus@2.13.7`, `axios@1.17.0`, `vue@3.5.33`, `vue-i18n@11.4.0`.
- The `add-full-url-helper` change is in proposal state, planning `fullURL` in `web/src/utils/common.ts`. Same function as buildadmin's `fullUrl`; lowercase name differs.
- No backend `/admin/ajax/upload` handler exists yet. Uploads will 404 until a separate backend change lands.

## Goals / Non-Goals

**Goals:**
- Port the buildadmin `baUpload.vue` + `selectFile.vue` to `agInput/components/`, replacing every `ba` / `baInput` reference with `ag` / `agInput`.
- Provide the 5 missing utility functions in `web/src/utils/common.ts` (`fullUrl`, `arrayFullUrl`, `getFileNameFromPath`, `getArrayKey`, `stringToArray`).
- Provide `fileUpload` in `web/src/api/common.ts` that posts to `/admin/ajax/upload` via the project's pre-configured axios instance.
- Provide zh-cn / en i18n keys for the component's user-visible strings.
- Provide `/agInput` admin route + `web/src/views/agInput/index.vue` demonstrating all 4 types.

**Non-Goals:**
- Adding the backend `/admin/ajax/upload` handler — out of scope; documented as a separate change the user can apply later.
- Building a "uploads library" feature (the `selectFile` dialog just opens a list of previously-uploaded files; backend list endpoint is also out of scope).
- Replacing buildadmin's deprecated `attr` prop behaviour; per user instruction, do not implement it.

## Decisions

### D1 — Mirror the reference file structure: `agInput/components/`, not `agInput/`

The buildadmin layout is `baInput/components/{baUpload,selectFile}.vue`. We mirror at `agInput/components/{agUpload,selectFile}.vue`. This keeps the future plan "port other buildadmin inputs (array / editor / iconSelector / remoteSelect)" trivial — each becomes a sibling file. Putting them flat under `agInput/` would prevent future sibling additions without moving files.

### D2 — Reuse project's pre-configured axios instance instead of buildadmin's `createAxios`

`web/src/utils/request.ts` already exports a configured axios instance with token injection, dedup, loading, and uniform `{code, message, data}` envelope unwrapping. Buildadmin uses `createAxios({url, method, data}, opts)` with two args (config + UI opts). We adapt `fileUpload` to call `request({url, method: 'POST', data: fd, params, onUploadProgress, __opts: {showErrorMessage: false, showHttpErrorMessage: false}})` to keep error UX consistent (component shows own ElMessage on fail).

### D3 — Drop the deprecated `attr` prop entirely

Buildadmin's `attr` prop is deprecated since v2.2.0 and slated for deletion. The reference itself logs a `console.warn` when used. Per user instruction "原组件已标记废弃的 `props` 无需实现", we omit `attr`. Consumer passes element-plus props directly to ag-upload.

### D4 — Co-locate `stringToArray`, `getFileNameFromPath`, `getArrayKey` in `web/src/utils/common.ts`; `fullUrl` / `arrayFullUrl` deferred to `add-full-url-helper`

`stringToArray` parses a `string | string[]` value into a string array (handles comma-separated). `getFileNameFromPath` returns the basename. `getArrayKey` finds an object's index by key/value. These three are small, reusable, and unrelated to URL composition.

`fullUrl` / `arrayFullUrl` ARE the responsibility of the in-flight `add-full-url-helper` change. To avoid duplication, the apply order is:
1. Apply `add-full-url-helper` first (lands `fullURL` in `common.ts`).
2. Then apply `add-ag-upload-component`; its tasks will `import { fullURL as fullUrl, fullURLArray as arrayFullUrl }` from `common.ts` — i.e. rename on import to keep buildadmin's camelCase naming local to agUpload.

If the user prefers to ship ag-upload without applying the URL helper, the fallback (recorded in `tasks.md`) is to inline `fullUrl` / `arrayFullUrl` in `common.ts` as part of this change, since they're tiny. The proposal marks this as an explicit decision branch.

### D5 — `fileUpload` returns the raw `ApiResponse<T>`, not a stripped payload

Buildadmin's `fileUpload` returns `ApiPromise` (i.e., `Promise<ApiResponse<...>>`). The component reads `res.data.file.url`. We keep the same shape so component code reads naturally: `const res = await fileUpload(fd); if (res.code === 0) file.serverUrl = res.data.file.url`. Returning the envelope verbatim also means the project's response interceptor still does dedup / loading cleanup on the upload request.

### D6 — Use `defineOptions({ inheritAttrs: false })` + `useAttrs()` to split events from el-upload props

The reference uses this pattern to (1) capture `onChange` / `onRemove` / etc. from `useAttrs()` and reroute them through `state.events` so they survive el-upload's internal re-render, and (2) forward remaining attrs as `v-bind="state.attrs"` on el-upload. We mirror this; it's the only sane way to expose el-upload's full event surface without fighting vue's prop fallthrough.

### D7 — Sortable only initialised when the list has ≥ 2 items AND `showFileList !== false`

Buildadmin re-creates the Sortable instance every time `onChange` fires (with a guard for `showFileList === false`). The component re-renders el-upload's list DOM after every status change; the Sortable instance must be re-attached. We replicate this with `initSort()` called from `onMounted` and `onChange`. The threshold "≥ 2 items" avoids useless Sortable binding for single-file modes.

### D8 — Inline clipboard `read()` with feature-detection; no fallback download

`navigator.clipboard.read()` requires HTTPS + user gesture. We don't fallback to `paste` event because the trigger is a click on the screenshot button (a gesture), so the API is always available if defined. When undefined (insecure context / older browser), we surface a warning. The `paste` event handler covers the keyboard-paste path independently.

### D9 — `agInput/index.vue` registers `/agInput` under the admin layout

The admin layout prefix is `adminBaseRoutePath` (`/admin`). We register `/agInput` under that prefix so the page sits alongside other admin pages and inherits the sidebar / auth guard. The test page is intentionally minimal: four ag-upload instances bound to four `ref('')` / `ref([])` state slots, with a hint banner explaining "POST /admin/ajax/upload not yet implemented; uploads will fail until backend lands".

## Risks / Trade-offs

- **[Risk] `add-full-url-helper` blocks this change.** Apply order matters: if `add-full-url-helper` is not applied first, `fullUrl` is missing. → **Mitigation:** D4 fallback inlines `fullUrl` / `arrayFullUrl` in `common.ts` directly. tasks.md documents this branch explicitly.
- **[Risk] `/admin/ajax/upload` returns 404** until the backend handler lands. → **Mitigation:** the test page makes this obvious; tasks.md calls out the dependency. Backend work is tracked separately.
- **[Risk] Sortable instance leak on unmount.** The reference never destroys Sortable instances. → **Mitigation:** keep the reference's behaviour; the component's lifetime matches its parent's. If a memory leak is later observed, swap to `onScopeDispose(() => sortable.destroy())` — but this is out of scope here.
- **[Risk] el-upload's internal list re-renders break Sortable's drag handle.** The reference calls `initSort()` from `onChange` and after `onMounted`, which covers re-render. → **Mitigation:** same as buildadmin; if breakage occurs in practice, the bug fix lives in a follow-up.
- **[Risk] `useAdminInfo()` called inside `fileUpload` throws during SSR / outside a setup context.** → **Mitigation:** wrap the `useAdminInfo()` access in try/catch and silently skip token injection (matches the request.ts interceptor's pattern).

## Migration Plan

No migration. This is a pure addition: 2 new components, 1 new API function, 5 new utility functions, 10 new i18n keys, 1 new test page + route. Rollback = `git revert`.

## Open Questions

None. The `fullUrl` import-vs-inline branch is recorded as D4 + a tasks.md branch; the user picks at apply time.