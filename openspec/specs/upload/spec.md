# Upload Specification

## Purpose

Provides a configurable, multi-driver file upload foundation so business code can persist user-uploaded files through a single entry point without re-implementing size / suffix / naming rules. The first driver is a local-disk implementation; future drivers (object storage, OSS, S3) plug in behind the same Driver interface without changing the call site.

## Requirements

### Requirement: Upload configuration source

The system SHALL load upload settings from `config/upload.yaml`, merged by viper with the existing per-file load order; `.env.yaml` MAY override any key. When that file is absent the subsystem SHALL fall back to built-in defaults.

#### Scenario: Default config is applied when upload.yaml is missing
- **WHEN** the system starts up without `config/upload.yaml`
- **THEN** the upload subsystem SHALL still function with these defaults: `driver=local`, `max_size=10`, `max_size_unit=MB`, `suffixes=["jpg","jpeg","png","gif","webp","pdf"]`, `format="/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}"`

### Requirement: Upload configuration knobs

The configuration SHALL expose four knobs: `driver`, `max_size`, `max_size_unit` and `suffixes`. `driver` SHALL name a registered storage driver and SHALL be rejected at startup when unknown. `max_size` SHALL be a positive integer expressed in `max_size_unit`, which SHALL be one of `B`, `KB`, `MB` or `GB`. `suffixes` SHALL list allowed extensions, case-insensitive and without a leading dot.

#### Scenario: Oversized file is rejected
- **WHEN** a file larger than `max_size × max_size_unit` bytes is uploaded
- **THEN** `Upload` SHALL reject it

#### Scenario: Disallowed suffix is rejected
- **WHEN** a file whose extension is not in `suffixes` is uploaded
- **THEN** `Upload` SHALL reject it

#### Scenario: Default suffix whitelist
- **WHEN** `suffixes` is not configured
- **THEN** the whitelist SHALL be `jpg`, `jpeg`, `png`, `gif`, `webp`, `pdf`

#### Scenario: Unknown driver name fails fast
- **WHEN** `config/upload.yaml` sets `driver: "s3"` but no S3 driver is registered
- **THEN** `upload.Init()` SHALL return an error mentioning the unknown driver name
- **AND** the process SHALL refuse to start (cmd/serve surfaces the error)

### Requirement: Upload storage path template

The `format` knob SHALL be a storage path template rendered at upload time. Multiple placeholders MAY appear in the same template, and the same placeholder MAY appear more than once.

#### Scenario: Default template
- **WHEN** `format` is not configured
- **THEN** the template SHALL be `/{topic}/{year}{mon}{day}/{fileName}{fileSha1}{.suffix}`

#### Scenario: Custom format produces deterministic paths
- **WHEN** an upload happens on 2026-09-30 with topic=`avatar`, original name=`My Photo.JPG`, content SHA1 prefix `3a7f...`, extension `jpg`
- **THEN** the rendered storage path SHALL be `/avatar/20260930/my photo3a7f...jpg`

### Requirement: Upload template placeholders

The template SHALL support these placeholders:

| Placeholder | Substitution |
| --- | --- |
| `{topic}` | the upload topic |
| `{year}` | 4-digit current year |
| `{mon}` | 2-digit current month |
| `{day}` | 2-digit current day |
| `{fileName}` | sanitized original file name |
| `{fileSha1}` | first 16 hex characters of the content SHA1 |
| `{.suffix}` | lowercased extension, leading dot included |

#### Scenario: Each placeholder occurrence is substituted independently
- **WHEN** a template contains `{topic}`, `{year}` and two occurrences of `{.suffix}`
- **THEN** every occurrence SHALL be substituted with its own value in place
- **AND** placeholders absent from the template SHALL NOT be emitted

### Requirement: Driver interface contract

The system SHALL define a `Driver` interface in the upload package with exactly five methods: `Save(ctx, content, storedPath) error`, `Delete(ctx, storedPath) error`, `Url(storedPath) string`, `Exists(storedPath) bool` and `FullPath(storedPath) string`. `upload.Init()` SHALL refuse to start when the configured driver name matches no registered driver.

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

#### Scenario: Local driver Exists reports on-disk presence
- **WHEN** `Exists(storedPath)` is called for a path that is and is not present on disk
- **THEN** it SHALL return `true` and `false` respectively

#### Scenario: Local driver FullPath joins base dir with storedPath
- **WHEN** the configured base directory is `/var/data/uploads` and `storedPath` is `avatar/20260930/x3a7f.jpg`
- **THEN** `FullPath(storedPath)` SHALL return `/var/data/uploads/avatar/20260930/x3a7f.jpg`

### Requirement: Service.Upload validates and persists

The system SHALL expose `Service.Upload(input *UploadInput) (*UploadResult, error)`, which validates the input, enforces the size cap and suffix whitelist, computes the content SHA1, renders the `format` template, persists the bytes through the configured driver, and returns the stored path, URL, byte count and suffix.

#### Scenario: Nil input is rejected
- **WHEN** `Upload` is called with `nil`, with `Reader == nil`, or with an empty `Topic`
- **THEN** it SHALL return `ErrInvalidInput` without touching the driver

#### Scenario: Empty original name is rejected
- **WHEN** `Upload` is called with an empty `OriginalName`
- **THEN** it SHALL return `ErrInvalidInput`

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

#### Scenario: Byte counter reflects consumed bytes
- **WHEN** the reader yields more bytes than any pre-declared size
- **THEN** the size cap SHALL be enforced against the actual bytes consumed

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
