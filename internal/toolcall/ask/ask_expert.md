# ask_expert

Ask expert for help.

Ask an expert to review plans or answer difficult questions.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed,

Parameters:

- content: detailed question (provide content or content_file, not both)

- content_file: path to a file containing the detailed question. The file is
  read directly from disk, so the exact content is sent — no LLM transcription
  drift. Safe paths only (current directory and subdirectories, max 1MB).

- summary: brief summary (optional)

- attachments: file attachments list (optional). Images (png/jpg/gif/webp/
  bmp) are uploaded to the web chat and analyzed visually; other files are
  inlined as text (max 1MB each, safe paths, up to 50 files/100MB).

- mode: web chat mode (optional). "pro" (expert, default), "flash" (fast
  with smart search), "vision" (image uploads). Empty: vision if images
  attached, else pro.

- role: role name for the system prompt (optional, default "expert"). Uses the
  prompt override chain: .dscli/prompt/<role>.md, ~/.dscli/prompt/<role>.md,
  role_configs mapping, built-in template. Ignored when system is provided.

- system: full system prompt text (optional). Completely replaces the default
  role template — use this for custom personas (e.g. a domain teacher).

- timeout: timeout in seconds (default 600). Set longer for complex questions
  requiring deep analysis.

Use for technical difficulties, plan review, or in-depth analysis.
