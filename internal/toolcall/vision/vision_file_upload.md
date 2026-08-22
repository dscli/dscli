# vision_file_upload

Upload a local image file to DeepSeek Files API and get back a `file_id`
(形如 `file-api-xxxxxxxxxxxxxxxx`).

The `file_id` refers to the image in subsequent chat requests — pass it to the
assistant as part of the conversation. The assistant runtime automatically
attaches newly uploaded files to the next user turn so the vision model can see
them.

**Use cases**: local image files that need to be analyzed by
`deepseek-v4-flash-vision-exp`; images too large for inline base64 (up to 64 MiB
via file_id vs 32 MiB inline); images reused across multiple requests.

**Limits**: JPEG/PNG/GIF/WebP (detected by content, not extension); 64 MiB max;
optional expiry 3600-2592000 seconds (1 hour to 30 days, default permanent).

**Parameters**:
- `file` (required): local image file path.
- `expires_seconds` (optional): validity in seconds, 3600-2592000. Empty means
  the file never expires.

**Returns** (JSON): `id`, `filename`, `bytes`, `created_at`, `purpose`,
`expires_at` (if set).
