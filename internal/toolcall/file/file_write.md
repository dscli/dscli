# write_file

Write content to file.

append=true appends, append=false overwrites (default).
Content max 524288 chars; split into multiple calls for
content larger than 524288 chars.

context (default true): after writing, returns a context
window showing the file state around the edit. Set false
to suppress and save output tokens.
