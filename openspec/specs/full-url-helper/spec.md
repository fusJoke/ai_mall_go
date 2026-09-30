# Full URL Helper Specification

## Purpose

Lets the backend and frontend convert a relative or driver-returned resource path into a fully-qualified URL that can be rendered, fetched, or shared, while preserving base64 payloads and external protocol links verbatim and honoring an optional CDN rewrite.

## Requirements

### Requirement: Resource URL builder accepts relative path, base64, and protocol-prefixed input

The system SHALL provide a URL builder that accepts a resource string which may be empty, a base64 data URI (`data:` prefix), a protocol-prefixed absolute URL (`http://` or `https://`), or a plain relative path. When the input is empty, a data URI, or a protocol-prefixed URL, the builder MUST return it unchanged.

#### Scenario: Empty resource
- **WHEN** the builder is called with an empty string
- **THEN** it returns an empty string

#### Scenario: Base64 data URI
- **WHEN** the builder is called with a string starting with `data:`
- **THEN** it returns the input unchanged

#### Scenario: HTTPS absolute URL
- **WHEN** the builder is called with a string starting with `https://`
- **THEN** it returns the input unchanged

#### Scenario: HTTP absolute URL
- **WHEN** the builder is called with a string starting with `http://`
- **THEN** it returns the input unchanged

### Requirement: Resource URL builder prepends current origin when no CDN is configured

The system SHALL provide a URL builder that, when the input is a plain relative path and no CDN rewrite is configured, prepends the current request origin (scheme + host + optional port) to the resource.

#### Scenario: Relative path with HTTP request
- **WHEN** the builder is called with a relative path and no CDN configured, and the request is HTTP to `api.example.com:8080`
- **THEN** it returns `http://api.example.com:8080<resource>`

#### Scenario: Relative path with HTTPS request
- **WHEN** the builder is called with a relative path and no CDN configured, and the request is HTTPS to `api.example.com`
- **THEN** it returns `https://api.example.com<resource>`

### Requirement: Resource URL builder prefers CDN origin when configured

The system SHALL provide a URL builder that, when a CDN base URL is configured and the input is a plain relative path, returns the CDN base URL plus any configured CDN path suffix plus the resource. The CDN configuration MUST be considered present when the configured base URL is a non-empty string.

#### Scenario: CDN base URL with no path suffix
- **WHEN** the builder is called with a relative path and the CDN base URL is `https://cdn.example.com` with an empty path suffix
- **THEN** it returns `https://cdn.example.com<resource>`

#### Scenario: CDN base URL with path suffix
- **WHEN** the builder is called with a relative path and the CDN base URL is `https://cdn.example.com` with the path suffix `format/heif`
- **THEN** it returns `https://cdn.example.com/format/heif<resource>`

#### Scenario: CDN takes precedence over current origin
- **WHEN** the builder is called with a relative path and the CDN base URL is configured
- **THEN** the result uses the CDN base URL and MUST NOT include the current request origin

### Requirement: Backend exposes the URL builder through the kit/urlx package

The backend MUST expose the URL builder as `kit/urlx.FullURL(c *gin.Context, resource string) string`, where `c` supplies the current request scheme and host. The function MUST read the CDN base URL and CDN path suffix from application configuration on each call (no caching of the origin).

#### Scenario: Backend kit/urlx FullURL with CDN configured
- **WHEN** a Go handler invokes `kit/urlx.FullURL(c, "/uploads/avatar/x.jpg")` with the CDN base URL `https://cdn.example.com` and path suffix `format/heif` configured
- **THEN** it returns `https://cdn.example.com/format/heif/uploads/avatar/x.jpg`

### Requirement: Frontend exposes the URL builder through utils/common.ts

The frontend MUST expose the URL builder as `fullURL(resource: string): string` in `web/src/utils/common.ts`, mirroring the backend's three branches (return-as-is for base64 / protocol, CDN rewrite when `cdnUrl` is configured, current origin otherwise). The CDN base URL MUST be sourced from the config Pinia store at `useConfig().cdnUrl`; the current origin MUST be sourced from `getBaseUrlPort()` in `web/src/utils/request.ts`.

#### Scenario: Frontend fullURL with cdnUrl configured
- **WHEN** a Vue component calls `fullURL("/uploads/avatar/x.jpg")` and `useConfig().cdnUrl` is `https://cdn.example.com`
- **THEN** it returns `https://cdn.example.com/uploads/avatar/x.jpg`

#### Scenario: Frontend fullURL with no cdnUrl
- **WHEN** a Vue component calls `fullURL("/uploads/avatar/x.jpg")` and `useConfig().cdnUrl` is empty and `getBaseUrlPort()` returns `http://api.example.com:8080`
- **THEN** it returns `http://api.example.com:8080/uploads/avatar/x.jpg`

#### Scenario: Frontend fullURL with base64 input
- **WHEN** a Vue component calls `fullURL("data:image/png;base64,iVBOR...")`
- **THEN** it returns the input unchanged

### Requirement: Configuration schema adds cdn_url and cdn_url_params

The configuration schema MUST add two new fields under the `server` block: `cdn_url` (CDN base URL, MUST NOT have a trailing slash) and `cdn_url_params` (path suffix appended between the CDN base URL and the resource). Both fields MUST be optional; absence or empty value MUST disable CDN rewriting.

#### Scenario: Default config has empty CDN fields
- **WHEN** the configuration is loaded with no `cdn_url` or `cdn_url_params` set
- **THEN** both fields are empty strings and the URL builder falls through to the current-origin branch