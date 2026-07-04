# send_message

Dispatch a message to another dscli chat session for a given project.

## When to use

- You need to communicate a finding, request, or instruction to another AI working on a different project
- You want to start a new task in a separate dscli session without blocking your current work
- The target project has a different AI persona assigned

## Parameters

- `input` (required): the message content to send
- `project` (required): absolute path to the target project directory

## Behavior

The tool is **IDE-agnostic** and **fire-and-forget**:

1. Writes the message to the target project's chimeins queue in the global database
2. If a dscli chat session is already running for the target project, it picks up the message in its next round
3. Otherwise, dispatches via a configurable display command (`send-message.command`) that starts a visible dscli session in the user's IDE

### Display command

Configured via `send-message.command` in `~/.dscli/config.dscli`:

```
send-message.command = emacsclient -n -c -e '(dscli--send-message-raw "%s" "%s")'
```

The `%s` placeholders are replaced with the message content and project path (shell-safe escaping applied automatically).

Default auto-detection order:
- `emacsclient` → `emacsclient -n -c -e '(dscli--send-message-raw ...)'`

### IDE integration

- **Emacs**: the `dscli--send-message-raw` function in dscli.el handles both running-session injection and new-session startup. The output buffer is displayed in the Emacs frame created by `-c`.
- **Other IDEs**: configure `send-message.command` for your environment (VSCode, terminal, etc.)
- **No display**: if no display command is configured and no auto-detect succeeds, the message is queued — run `dscli chat` manually in the target project to see it.

## Example

```
send_message(input="请 review dscli-main.el 的改动", project="/home/user/project/dscli.el")
```
