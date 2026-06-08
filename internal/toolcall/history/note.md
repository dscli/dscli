<!-- Max note length is defined in prompt.MaxNoteContentLen; keep in sync -->
# note

Summarize session for future recall.

Record a key summary of the current conversation. Call at the
end of a conversation.

Content must be 120 characters or less. Notes are short retrieval
clues for recall — not long-term storage. Use mem_save for detailed
records, configuration, decisions, and patterns.

Rejected if over 120 characters — edit down to key events and
keywords (e.g., "Implemented recall tool with session_id filter").
