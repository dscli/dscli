# vision_file_delete

Delete an uploaded file by its `file_id`. Use when the file is no longer
needed (storage quota: 25 GiB / 10000 files per user).

**Parameters**:
- `file_id` (required): the id returned by `vision_file_read`.

**Returns** (JSON): `id`, `object`, `deleted`.
