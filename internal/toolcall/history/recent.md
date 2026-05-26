# recent

List recent messages for the current session.

List messages (user and assistant without tool_calls), ordered by
created_at ascending (newest at bottom). Returns a table with id,
time, role, and truncated content preview.

Parameters:

- limit: max results (default 20, max 20)
