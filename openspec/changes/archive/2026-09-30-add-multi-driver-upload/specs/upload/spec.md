# Spec Delta

## Purpose

Provides a configurable, multi-driver file upload foundation so business code can persist user-uploaded files through a single entry point without re-implementing size / suffix / naming rules. The first driver is a local-disk implementation; future drivers (object storage, OSS, S3) plug in behind the same Driver interface without changing the call site.

## ADDED Requirements

### Requirement: Upload configuration

The system SHALL load upload settings from `config/upload.yaml` (merged by viper with the existing per-file load order; `.env.yaml` may override any key). The configuration SHALL expose four knobs:

- `driver`: string — name of the storage driver to use. The system SHALL reject any unknown driver name at startup.
- `max_size`: positive integer — maximum file size in `max_size_unit`. A file larger than `max_size × max_size_unit` bytes SHALL be rejected by `Upload`.
- `max_size_unit`: string in `{"B", "KB", "MB", "GB"}` — multiplier unit for `max_size`.
- `suffixes`: list of strings (case-insensitive, no leading dot) — allowed file extensions. A file with an extension not in the list SHALL be rejected by `Upload`. The default list SHALL include `jpg`, `jpeg`, `png`, `gif`, `webp`, `pdf`.
- `format`: string — storage path template. The system SHALL support the following placeholders and substitute them at upload time:

| Placeholder | Substitution |
| --- | --- |
| `{topic}` | the value passed as the upload topic |
| `{year}` | 4-digit current year |
| `{mon}` | 2-digit current month |
| `{day}` | 2-digit current day |
| `{fileName}` | sanitized original file name |
| `{fileSha1}` | first 16 hex characters of SHA1 of file contents |
| `{.suffix}` | the lowercased file extension including the leading dot |

Multiple placeholders MAY appear in the same template; the same placeholder MAY appear more than once. The default template SHALL be `/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}`.

#### Scenario: Default config is applied when upload.yaml is missing
- **WHEN** the system starts up without `config/upload.yaml`
- **THEN** the upload subsystem SHALL still function with these defaults: `driver=local`, `max_size=10`, `max_size_unit=MB`, `suffixes=["jpg","jpeg","png","gif","webp","pdf"]`, `format="/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}"`

#### Scenario: Unknown driver name fails fast
- **WHEN** `config/upload.yaml` sets `driver: "s3"` but no S3 driver is registered
- **THEN** `upload.Init()` SHALL return an error mentioning the unknown driver name
- **AND** the process SHALL refuse to start (cmd/serve surfaces the error)

#### Scenario: Custom format produces deterministic paths
- **WHEN** an upload happens on 2026-09-30 with topic=`avatar`, original name=`My Photo.JPG`, content SHA1 prefix `3a7f...`, extension `jpg`
- **THEN** the rendered storage path SHALL be `/avatar/20260930/my photo3a7f...jpg`

### Requirement: Driver interface contract

The system SHALL define a `Driver` interface in the upload package with exactly five methods:

- `Save(ctx context.Context, content io.Reader, storedPath string) error` — persist the bytes at the driver-specific location identified by `storedPath` (the rendered template); create any missing parent directories.
- `Delete(ctx context.Context, storedPath string) error` — remove the stored file; idempotent (no error when the file is already absent).
- `Url(storedPath string) string` — return the externally addressable URL for accessing the file (for `local` driver, the public-prefixed HTTP path).
- `Exists(storedPath string) bool` — return whether the file currently exists in storage.
- `FullPath(storedPath string) string` — return the absolute filesystem path on disk (for `local` driver); for non-local drivers this SHALL be the canonical storage identifier.

The system SHALL refuse to start `upload.Init()` if the configured driver name does not match any registered driver.

#### Scenario: Local driver Save creates missing parent directories
- **WHEN** `Save` is invoked with a `storedPath` whose parent directory does not exist
- **THEN** the driver SHALL create the directory tree with mode 0o755 before writing
- **AND** return nil on success

#### Scenario: Local driver Delete is idempotent
- **WHEN** `Delete` is invoked for a stored path that does not exist on disk
- **THEN** the driver SHALL return nil without raising an error

#### Scenario: Local driver Url returns public-prefixed path
- **WHEN** the configured public URL prefix is `/uploads` and `storedPath` is `avatar/20260930/x3a7f.jpg`
- **THEN** `Url(storedPath)` SHALL return `/uploads/avatar/20260930/x3a7f.jpg`

#### Scenario: Local driver FullPath joins base dir with storedPath
- **WHEN** the configured base directory is `/var/data/uploads` and `storedPath` is `avatar/20260930/x3a7f.jpg`
- **THEN** `FullPath(storedPath)` SHALL return `/var/data/uploads/avatar/20260930/x3a7f.jpg`

### Requirement: Service.Upload validates and persists

The system SHALL expose a `Service.Upload(input *UploadInput) (*UploadResult, error)` that, in order:

1. Returns `ErrInvalidInput` if `input == nil`, `input.Reader == nil`, or `input.Topic == ""`.
2. Returns `ErrInvalidInput` if `input.OriginalName == ""`.
3. Reads up to `max_size × max_size_unit` bytes from `input.Reader`; if the read exceeds the cap, returns `ErrFileTooLarge`. The byte counter SHALL count actual bytes consumed, not pre-declared sizes.
4. Determines the lowercased extension from `input.OriginalName`; if absent or not in `suffixes`, returns `ErrInvalidSuffix`.
5. Computes SHA1 over the bytes read in step 3.
6. Renders the configured `format` template with the substitution table above.
7. Calls `driver.Save` with the rendered path and a reader over the buffered bytes (rewound to the start).
8. Returns `&UploadResult{StoredPath: rendered, Url: driver.Url(rendered), Size: byteCount, Suffix: ext}`.

#### Scenario: File too large is rejected before disk write
- **WHEN** a 12 MB file is uploaded against a `max_size=10 MB` configuration
- **THEN** `Upload` SHALL return `ErrFileTooLarge`
- **AND** the driver SHALL NOT have been called

#### Scenario: Disallowed extension is rejected
- **WHEN** an `.exe` file is uploaded against the default suffix whitelist
- **THEN** `Upload` SHALL return `ErrInvalidSuffix`
- **AND** the driver SHALL NOT have been called

#### Scenario: Successful upload returns url and stored path
- **WHEN** a 2 MB `.jpg` file with topic=`avatar` is uploaded successfully
- **THEN** the returned `UploadResult.Url` SHALL be the driver's `Url` of the rendered path
- **AND** the returned `UploadResult.StoredPath` SHALL match the rendered template output
- **AND** `UploadResult.Size` SHALL equal 2 MB

### Requirement: File suffix and image detection helpers

The system SHALL expose two helpers on `Service`:

- `GetSuffix(filename string) string` — return the lowercase extension without the leading dot; return empty string if there is no extension.
- `IsImage(filename string) bool` — return `true` if the lowercased extension is in the set `{jpg, jpeg, png, gif, webp, bmp, svg, tiff}`; otherwise `false`.

#### Scenario: Mixed-case extension is normalized
- **WHEN** `GetSuffix("Photo.JPG")` is called
- **THEN** it SHALL return `"jpg"`

#### Scenario: Filename with no extension
- **WHEN** `GetSuffix("README")` is called
- **THEN** it SHALL return `""`

#### Scenario: IsImage recognises common raster formats
- **WHEN** `IsImage` is called with `"foo.png"` / `"foo.svg"` / `"foo.pdf"`
- **THEN** it SHALL return `true`, `true`, `false` respectively

### Requirement: Sentinel errors

The system SHALL expose three sentinel errors from the upload package: `ErrInvalidInput`, `ErrFileTooLarge`, `ErrInvalidSuffix`. Drivers MAY return additional driver-specific errors (wrapped with `fmt.Errorf("...: %w", err)`); the system SHALL NOT silently swallow them.

#### Scenario: Errors are distinguishable via errors.Is
- **WHEN** a caller does `errors.Is(err, upload.ErrFileTooLarge)`
- **THEN** it SHALL return `true` exactly when `Upload` rejected the file for size
- **AND** `false` for other failure modes
