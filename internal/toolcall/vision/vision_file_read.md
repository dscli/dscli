# vision_file_read

Read (upload) a local image into the conversation so the vision model can
see it, and get back a `file_id` (形如 `file-api-xxxxxxxxxxxxxxxx`).

The `file_id` refers to the image in subsequent chat requests. Unlike a
plain upload, the assistant runtime **injects the image as a user message
in the same round**: the vision model sees it in the very next model turn,
without waiting for the next user turn. Non-vision models get only the
metadata JSON (image blocks are user-message-only in the OpenAI protocol).

**Use cases**: verify a user-attached image yourself (the intent is "read
this image so I can see it", not "upload to cloud storage"); images too
large for inline base64 (up to 64 MiB via file_id vs 32 MiB inline);
images reused across multiple requests.

**Note**: the same local file (same size + SHA-256, see `~/.dscli/files.json`)
is cached — repeated reads return the existing `file_id` with no network
request.

**Limits**: JPEG/PNG/GIF/WebP (detected by content, not extension); 64 MiB max;
optional expiry 3600-2592000 seconds (1 hour to 30 days, default permanent).

**Parameters**:
- `file` (required): local image file path.
- `expires_seconds` (optional): validity in seconds, 3600-2592000. Empty means
  the file never expires.

**Returns** (JSON): `id`, `filename`, `bytes`, `created_at`, `purpose`,
`expires_at` (if set).
