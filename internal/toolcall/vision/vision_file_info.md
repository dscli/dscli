# vision_file_info

Get metadata for a single uploaded file by its `file_id`.

**Parameters**:
- `file_id` (required): the id returned by `vision_file_upload`.

**Returns** (JSON): `id`, `filename`, `bytes`, `created_at`, `purpose`,
`expires_at` (if set).
