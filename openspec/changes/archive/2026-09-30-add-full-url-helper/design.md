# Design

## Context

`internal/kit/` exists (per CLAUDE.md) for cross-cutting business-layer helpers. It currently has no Go files. The new `FullURL` helper depends on `gin.Context` for the request origin and on `internal/infra/config` for the CDN configuration; neither dependency is appropriate for `pkg/`, which must stay framework-agnostic.

On the frontend, `web/src/utils/request.ts` already exposes `getBaseUrlPort()` (scheme + host + port derived from `import.meta.env.VITE_AXIOS_BASE_URL`), and `web/src/stores/config.ts` is the existing Pinia store for site-wide configuration. The new helper reuses both.

The upload driver layer (`internal/infra/upload`) keeps returning relative paths (e.g. `/uploads/avatar/...`); the new helper sits one level above the driver and is called by handlers when shaping API responses.

## Goals / Non-Goals

**Goals:**
- One symmetric helper per runtime (Go + TS) with identical branching rules.
- Configuration-driven CDN with zero-code swap-back to origin.
- Zero allocation cost on the common path (no string copy when input is base64 / protocol-prefixed).

**Non-Goals:**
- Not introducing a signing / pre-signed URL flow (out of scope for this change).
- Not migrating existing handler responses to use the helper. Wiring is left to each feature change so this change stays a pure infrastructure add.

## Decisions

### D1 — Place the helper in `internal/kit/urlx`, not `pkg/urlx`

`pkg/` is reserved for framework-agnostic libraries that external projects could import. `FullURL` needs `*gin.Context` (current request scheme / host) and `config.Get().Server` (CDN). Placing it under `pkg/` would force external consumers to depend on gin and our config loader — violating the package's contract.

`internal/kit/` is documented as the home for cross-cutting business helpers that can depend on internal packages. The dependency on gin is acceptable here because the kit is internal to this repo and not part of the public surface.

**Alternatives considered:**
- `pkg/urlx` — rejected: pulls gin + viper into the public package surface.
- `internal/infra/urlx` — rejected: `infra/` is for infrastructure drivers (database, token, upload, captcha); a pure-function helper doesn't fit.
- Top-level `internal/urlx` — works but loses the "kit = cross-cutting helper" semantic grouping.

### D2 — Base64 detection by `data:` prefix only

A base64 image / file is encoded as `data:<mime>;base64,<payload>`. We detect it with `strings.HasPrefix(resource, "data:")` rather than full RFC 2397 parsing (which would require checking `;base64`, charset, etc). The "starts with `data:`" check is conservative — any future spec-compliant data URI passes through; the rare malformed input that starts with `data:` but isn't a URI is still harmless (returned as-is).

**Alternatives considered:**
- Regex match `^data:[a-zA-Z0-9+/]+;base64,` — rejected: overkill and ties us to a stricter syntax than RFC requires.

### D4 — Recognise `http://` and `https://` as pass-through

These are the two schemes a resource on the public internet realistically arrives in. Other schemes (`ftp://`, `mailto:`, `tel:`) are not expected as resource paths; if they appear, current origin prepending is harmless because no business path returns them.

### D5 — Current-origin detection from `c.Request.TLS` + `c.Request.Host`

Backend scheme detection:
1. If `c.Request.TLS != nil` → `"https"`
2. Else check `X-Forwarded-Proto` header (gin behind nginx / ALB)
3. Else `"http"`

Host comes from `c.Request.Host` directly — gin already populates it from the `Host` header, and it includes the port when the client sent one (`:8080`). No need to re-parse.

This produces strings like `https://api.example.com` or `http://api.example.com:8080`, which we then concatenate directly with the resource. No `net/url` parsing — it would mangle `+` characters in base64 and add unnecessary overhead.

### D6 — Read CDN config on every call (no caching)

The function is invoked at response-shaping time (millisecond cadence, one per resource). Reading `config.Get()` per call is fine because:
- `config.Get()` is a pointer return, not a struct copy.
- Caching the CDN URL inside `kit/urlx` would couple the helper to config-reload semantics that don't exist today.

### D7 — Frontend mirror uses `useConfig().cdnUrl` + `getBaseUrlPort()`

Frontend must mirror backend behavior. We deliberately avoid calling `window.location.origin` because:
- The API may be deployed behind a reverse proxy whose `Host` header differs from the user's URL bar.
- `getBaseUrlPort()` already encapsulates the env-var fallback logic.

`cdnUrl` is a plain string field on `useConfig`; no reactivity magic needed. The store is hydrated by `setSiteConfig(data)` after `GET /admin/init`, so the value is available by the time any view component renders an `avatar` / `cover`.

### D8 — Frontend `cdn_url_params` defaults to empty string; CDN prefix concatenation is identical to backend

To keep the two implementations aligned, the frontend reads `useConfig().cdnUrlParams` (mirrors backend) and concatenates `cdnUrl + cdn_url_params + resource`. When `cdn_url_params` is empty, the result is `cdnUrl + resource`. When `cdnUrl` is empty, the function falls through to the origin branch.

## Risks / Trade-offs

- **[Risk] Origin from reverse-proxy mis-match.** If gin is behind nginx without `X-Forwarded-Proto` being set, HTTPS traffic is reported as HTTP. → **Mitigation:** trust `c.Request.TLS` first (only true if TLS is terminated inside gin), then fall back to `X-Forwarded-Proto`. Operators are responsible for forwarding the header.
- **[Risk] Frontend `getBaseUrlPort()` returns empty string when env var is unset.** → **Mitigation:** `fullURL` treats empty origin as "use the resource as-is" rather than producing `"<resource>"` with no host. The frontend branch mirrors the backend: empty CDN → empty origin → relative path returned as-is (acceptable for SSR or static export).
- **[Risk] Path-normalisation edge cases** (resource starting with `/` vs not). → **Mitigation:** the spec guarantees the resource is either a base64 / protocol URI (returned verbatim) or a plain relative path; we concatenate without normalisation. Caller is responsible for ensuring the resource is leading-slash relative. Driver `Url` produces leading-slash relative paths, so this is consistent.

## Migration Plan

No migration needed. The change is additive:
- Existing handlers continue to return relative URLs (driver `Url()` output).
- New helper is opt-in per handler.

Rollback: remove the new files; existing code paths unaffected.

## Open Questions

None. All material decisions are resolved above.