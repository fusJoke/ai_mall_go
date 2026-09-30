# ag-upload Specification

## Purpose

Lets administrators upload images and files through a reusable Vue 3 component that supports click-to-select, drag-and-drop, clipboard paste, upload progress, file preview, drag-sort reordering, and a file picker dialog, while emitting a normalized URL list through `v-model`.

## Requirements

### Requirement: agUpload accepts a type prop that controls upload mode

The system MUST provide an `ag-upload` Vue component that accepts a `type` prop with one of four values: `image` (single image), `images` (multiple images, picture-card layout), `file` (single file), `files` (multiple files). When `type` is `image` or `images`, the component MUST render an image-only trigger with paste / drag-drop affordances; when `type` is `file` or `files`, the component MUST render a button-based trigger for arbitrary files.

#### Scenario: Single image mode
- **WHEN** the consumer renders `<ag-upload type="image" v-model="avatar" />`
- **THEN** the component shows a picture-card trigger restricted to image MIME types and accepts exactly one file

#### Scenario: Multiple images mode
- **WHEN** the consumer renders `<ag-upload type="images" v-model="gallery" />`
- **THEN** the component shows a picture-card trigger that allows adding multiple images

#### Scenario: Single file mode
- **WHEN** the consumer renders `<ag-upload type="file" v-model="doc" />`
- **THEN** the component shows a button trigger for any file type and accepts exactly one file

#### Scenario: Multiple files mode
- **WHEN** the consumer renders `<ag-upload type="files" v-model="docs" />`
- **THEN** the component shows a button trigger for any file type and allows adding multiple files

### Requirement: agUpload emits v-model with normalized URL list

The system MUST make the `ag-upload` component a two-way bound form field through `v-model`. When `modelValue` is a string, the component MUST emit a comma-joined string of uploaded file URLs; when it is an array, it MUST emit an array. When the consumer passes `returnFullUrl: true`, the component MUST transform each emitted URL into a fully-qualified URL via the `fullUrl` helper.

#### Scenario: Default v-model emits relative URLs
- **WHEN** the consumer binds `v-model` and two files finish uploading
- **THEN** the emitted value is a comma-joined string (or array) of the uploaded file server paths

#### Scenario: returnFullUrl true prepends origin
- **WHEN** the consumer binds `v-model` with `return-full-url` set to true
- **THEN** each emitted URL is rewritten via `fullUrl` so the consumer receives absolute URLs

### Requirement: agUpload uploads files through the shared fileUpload API

The system MUST post each selected file to the server via the `fileUpload` function exported from `web/src/api/common.ts`. The function MUST accept a `FormData` payload, optional extra params, and an `onUploadProgress` callback. The function MUST return a promise that resolves with the server response; if the response code is non-zero, the component MUST mark the file as failed and remove it from the visible list. If the file exceeds the configured maximum size or has a disallowed MIME type, the component MUST NOT submit the request and MUST surface a notification.

#### Scenario: Successful upload
- **WHEN** a user selects a valid file and `fileUpload` resolves with `code === 0`
- **THEN** the file appears in the list with status `success` and its returned URL is included in the next `update:modelValue` emission

#### Scenario: Server-side rejection
- **WHEN** a user selects a valid file and `fileUpload` resolves with a non-zero `code`
- **THEN** the file is removed from the visible list and the upload is marked as failed

#### Scenario: Oversize file
- **WHEN** a user selects a file larger than the configured `max_size`
- **THEN** the upload request is not sent and an error notification is shown

### Requirement: agUpload exposes drag-and-drop upload

The system MUST allow users to drop one or more files onto the upload area to start uploads. The component MUST filter dropped files by the active `type` (images for `image` / `images`, any type for `file` / `files`). If the drop contains no valid files, the component MUST display a localized warning.

#### Scenario: Drop images in images mode
- **WHEN** a user drags three image files onto the component while `type="images"`
- **THEN** the three images are added to the upload queue

#### Scenario: Drop non-images in images mode
- **WHEN** a user drags a PDF onto the component while `type="images"`
- **THEN** no file is added and a warning is shown

### Requirement: agUpload supports clipboard image paste

The system MUST allow users to paste an image from the clipboard into the upload area when `type` is `image` or `images`. Pasting non-image content in image mode MUST show a localized warning. The component MUST also expose a "screenshot upload" affordance that calls `navigator.clipboard.read()` for environments that support it.

#### Scenario: Paste image into image upload
- **WHEN** the user focuses the upload area and pastes an image
- **THEN** the image is added to the upload queue

#### Scenario: Paste non-image into image upload
- **WHEN** the user focuses the upload area and pastes text
- **THEN** a localized "no image in clipboard" warning is shown

#### Scenario: Clipboard API unavailable
- **WHEN** the user clicks the screenshot upload button and `navigator.clipboard.read` is not defined
- **THEN** a localized "clipboard read is not supported" warning is shown

### Requirement: agUpload reports upload progress

The system MUST report per-file upload progress while a request is in flight. The progress MUST be exposed as a percentage (0-100) on each file object in the file list and MUST update at least once per network progress event.

#### Scenario: Progress reaches 100
- **WHEN** a file is being uploaded and the network reports 50% then 100%
- **THEN** the file's `percentage` is updated to 50 and then to 100 before its status changes to `success`

### Requirement: agUpload supports drag-sort reordering

The system MUST allow users to drag items within the file list to reorder them when more than one item is present and `type` is `images` or `files`. Reordering MUST emit an updated `update:modelValue` reflecting the new order.

#### Scenario: Reorder images
- **WHEN** the user drags the second image above the first in `images` mode
- **THEN** the emitted `v-model` array is reordered to put the dragged image first

#### Scenario: Reorder disabled for single mode
- **WHEN** `type="image"` and only one file is in the list
- **THEN** drag-sort is not initialized

### Requirement: agUpload opens a file picker dialog when requested

The system MUST render a "select from library" affordance that opens the `selectFile` dialog component, unless the consumer opts out via `hideSelectFile`. The dialog MUST accept the consumer's `v-model` and append selected files to the upload list.

#### Scenario: Default shows the picker button
- **WHEN** the consumer does not pass `hideSelectFile`
- **THEN** the trigger area exposes a "choice" button that opens the picker dialog

#### Scenario: Picker hidden via prop
- **WHEN** the consumer passes `hideSelectFile`
- **THEN** no "choice" button is rendered

### Requirement: agUpload is fully renamed from the buildadmin baUpload reference

The system MUST NOT contain any of the `ba` prefix references carried over from the buildadmin source. Every class name, identifier, and comment that references `baUpload`, `ba-upload`, `baInput`, or related identifiers MUST be replaced with the `ag` equivalent (`agUpload`, `ag-upload`, `agInput`, etc.). Deprecated props from the reference (`attr`) MUST NOT be implemented.

#### Scenario: No ba- class names in the rendered DOM
- **WHEN** ag-upload is rendered
- **THEN** the rendered DOM contains `ag-upload-*` classes only, and no `ba-upload-*` class appears

#### Scenario: attr prop absent
- **WHEN** the consumer inspects the component's TypeScript props
- **THEN** the `attr` prop is not present and any consumer passing it receives a TypeScript error

### Requirement: agUpload is demonstrated at /agInput

The system MUST register a route at `/agInput` under the admin layout that renders a single page (`web/src/views/agInput/index.vue`) containing one `<ag-upload>` per supported type (`image`, `images`, `file`, `files`) with hard-coded initial model values for manual testing.

#### Scenario: Visiting /agInput
- **WHEN** a logged-in admin opens `/agInput`
- **THEN** the page renders four upload areas, one per type, each bound to its own local state
