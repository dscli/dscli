# wakeup

Wake up another AI maintainer at a given project, optionally with a message.

## When to use

- Communicate a finding, request, or instruction to another AI working on a
  different project
- Start a new task in a separate dscli session without blocking your current work
- You've already communicated via mail (sendmail) and just need to wake the AI
  up — `input` can be empty

## Parameters

- `project` (required): absolute path to the target project directory
- `input` (optional): the message content to send — empty is fine if you've
  already communicated details via mail
- `ainame` (optional): the AI maintainer name at the target project —
  validated against the project's assignment if provided

## Behavior

The tool is **IDE-agnostic** and **fire-and-forget**:

1. If `input` is provided, writes it to the target project's chimeins queue
2. If a dscli chat session is already running, it picks up the chimein
   in its next round
3. Otherwise, dispatches via `wakeup.command` (config) to start a session

### Display command

Configured via `wakeup.command` in `~/.dscli/config.dscli`. The `%s`
placeholder is replaced with the project path. Default auto-detection:

```
emacsclient -n -c -e '(dscli--send-message-raw "%s")'
```

### Ainame validation

When `ainame` is provided, the tool checks that it matches the target
project's assigned maintainer. This prevents LLM hallucination — if you
think Curie is on project X but Bohr is actually assigned, the tool
catches the mistake early.

## Examples

```
wakeup(input="请 review 我的改动", project="/home/user/project/dscli.el")
wakeup(project="/home/user/project/big-project")  // wake without message
```
