# ask_expert

Ask expert for help.

Ask an expert to review plans or answer difficult questions.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed,

Parameters:

- input: the question to ask (required). If the value starts with @ and
  points to an existing file (e.g. @question.txt), the file content is read
  directly from disk — no LLM transcription drift. Safe paths only (current
  directory and subdirectories, max 1MB). Anything else starting with @ is
  sent as plain text.

- attachments: file attachments list (optional). Images (png/jpg/gif/webp/
  bmp) are uploaded to the web chat and analyzed visually; other files are
  inlined as text (max 1MB each, safe paths, up to 50 files/100MB).

- mode: web chat mode (optional). "pro" (expert, default), "flash" (fast
  with smart search), "vision" (image uploads). Empty: vision if images
  attached, else pro.

- timeout: timeout in seconds (default 600). Set longer for complex questions
  requiring deep analysis.

Use for technical difficulties, plan review, or in-depth analysis.
