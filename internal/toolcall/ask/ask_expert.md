# ask_expert

Ask expert for help.

Ask an expert to review plans or answer difficult questions.
Uses DeepSeek Web (free V4 Pro) via Chrome browser — no API key needed.

Key behaviors (parameter details are in the tool schema):

- @-prefixed input reads the question from a file: safe paths only
  (cwd-relative, ~/..., $HOME-absolute, or /tmp; max 1MB). Anything else
  starting with @ is sent as plain text.
- Every result ends with "conversation_id: <id>": pass it back via keep
  to continue the SAME conversation — the expert keeps full history,
  including previously uploaded images.
- raw sends the input verbatim, skipping dscli's default response template
  (use when the prompt defines the output format, e.g. JSON extraction).

Correction flow example (misread image):
1. ask_expert(input="分析这张图", attachments=[img], mode="vision")
   → result ends with conversation_id: abc123
2. ask_expert(input="再仔细看，是不是白发罗小黑？", keep="abc123")
   → expert re-examines the SAME image in context, no re-upload

Use for technical difficulties, plan review, or in-depth analysis.
