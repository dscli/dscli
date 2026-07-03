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

The tool dispatches the message via Emacs daemon (emacsclient --eval).  

If the target project already has a running dscli chat session, the message
is injected into the existing session.  Otherwise, a new dscli chat session
is started for that project.

The call is fire-and-forget: the message is dispatched, and control returns
to the calling AI immediately.  The recipient AI processes the message
independently in its own session context.

## Example

```
send_message(input="请 review dscli-main.el 的改动", project="/home/user/project/dscli.el")
```
