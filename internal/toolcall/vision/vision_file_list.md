# vision_file_list

List files previously uploaded via `vision_file_upload`.

**Parameters** (all optional):
- `after`: pagination cursor — return files after this `file_id`.
- `limit`: number of files to return, 1-1000 (default 1000).
- `order`: sort by creation time — `asc` (default) or `desc`.

**Returns** (JSON): `object`, `data` (array of file objects), `first_id`,
`last_id`, `has_more`.
