# ag-upload Specification

## MODIFIED Requirements

### Requirement: agUpload uploads files through the shared fileUpload API

The system MUST post each selected file to the server via the `fileUpload` function exported from `web/src/api/common.ts`. The function MUST accept a `FormData` payload, optional extra params, and an `onUploadProgress` callback. The component MUST forward its optional `driver` prop (defaulting to `local`) as a `driver` extra param alongside the existing `force_local` param, letting the server decide the actual storage driver (falling back to local storage when the driver is unconfigured). The function MUST return a promise that resolves with the server response; if the response code is non-zero, the component MUST mark the file as failed and remove it from the visible list. If the file exceeds the configured maximum size or has a disallowed MIME type, the component MUST NOT submit the request and MUST surface a notification.

#### Scenario: Successful upload

- **WHEN** a user selects a valid file and `fileUpload` resolves with `code === 0`
- **THEN** the file appears in the list with status `success` and its returned URL is included in the next `update:modelValue` emission

#### Scenario: Server-side rejection

- **WHEN** a user selects a valid file and `fileUpload` resolves with a non-zero `code`
- **THEN** the file is removed from the visible list and the upload is marked as failed

#### Scenario: Oversize file

- **WHEN** a user selects a file larger than the configured `max_size`
- **THEN** the upload request is not sent and an error notification is shown

#### Scenario: Driver param forwarded

- **WHEN** the consumer renders `<ag-upload type="image" :driver="'qiniu'" />` and a file starts uploading
- **THEN** the upload request payload includes `driver=qiniu` as an extra param

#### Scenario: Default driver is local

- **WHEN** the consumer does not pass a `driver` prop and a file starts uploading
- **THEN** the upload request payload includes `driver=local`

### Requirement: agUpload is demonstrated at /agInput

The system MUST register a route at `/agInput` under the admin layout that renders a single page (`web/src/views/agInput/index.vue`) containing one `<ag-upload>` per supported type (`image`, `images`, `file`, `files`) with local model state for manual testing. The page MUST also render an upload-driver selector (example data: local / aliyun / tencent / qiniu cards from `web/src/mock/uploadDrivers.ts`) whose selected value is bound to the `driver` prop of all four upload areas, and MUST show the current model value of each upload area.

#### Scenario: Visiting /agInput

- **WHEN** a logged-in admin opens `/agInput`
- **THEN** the page renders four upload areas, one per type, each bound to its own local state

#### Scenario: Switching the upload driver

- **WHEN** the user selects the "七牛云 Kodo" driver card on `/agInput`
- **THEN** all four upload areas forward `driver=qiniu` on their next upload request, and the current-driver hint reflects the selection
