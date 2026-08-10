# ask_expert

Ask expert for help.

Ask an expert to review plans or answer difficult questions.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed,

Parameters:

- input: the question to ask (required). If the value starts with @ and
  points to an existing file (e.g. @question.txt, @~/notes/q.txt), the file
  content is read directly from disk — no LLM transcription drift. Safe
  paths only: cwd-relative, ~/..., $HOME-absolute, or /tmp (max 1MB).
  Anything else starting with @ is sent as plain text.

- attachments: image files (png/jpg/gif/webp/bmp) uploaded for visual
  analysis; other files inlined as text (1MB max, safe paths = cwd, $HOME,
  or /tmp; ≤50 files/≤100MB).

- mode: web chat mode. "pro" (expert, default), "flash" (fast with smart
  search), "vision" (image uploads). Empty: vision if images attached,
  else pro.

- keep: continue a previous conversation (default empty = new). Every
  result ends with "conversation_id: <id>"; pass that id back to continue
  the SAME conversation — the expert keeps full history, including
  previously uploaded images. "last" continues the most recent one. A full
  chat.deepseek.com URL (browser-opened) is accepted and registered.
  "list" lists saved conversations instead of asking.

- timeout: timeout in seconds (default 600). Set longer for complex
  questions requiring deep analysis.

- raw: send the input verbatim, skipping dscli's default response template
  (default false). Set true when the prompt itself defines the required
  output format (e.g. JSON extraction) — the standard Problem
  Analysis/Solutions/Suggestions boilerplate is then omitted.

Correction flow example (misread image):
  1. ask_expert(input="分析这张图", attachments=[img], mode="vision")
     → result ends with conversation_id: abc123
  2. ask_expert(input="再仔细看，是不是白发罗小黑？", keep="abc123")
     → expert re-examines the SAME image in context, no re-upload

Use for technical difficulties, plan review, or in-depth analysis.
