# wakeup

Wake up another AI maintainer at a given project, optionally with a message.

- `project` (required): absolute path to the target project directory
- `input` (optional): the message content to send — empty is fine if you've already communicated via mail
- `ainame` (optional): validated against the project's assignment if provided
- `role` (optional): persona of the woken session - `architect` (the task is
  still an idea: requirements and design need work), `dev` (idea and
  implementation are both settled, so no architect analysis is needed),
  `review`, `expert`, `test`. Omitted means the `chat` CLI default
  (architect).

```
wakeup(input="请 review 我的改动", project="/home/user/project/dscli.el", role="review")
wakeup(project="/home/user/project/big-project")  // wake without message
```

`role` only takes effect when a session is started. If a session is already
running for the target project, the message is delivered to it but the role
stays as it was (a session's role is fixed at startup; the message header
still records the requested role). The built-in Emacs wakeup path passes the
role through to `dscli chat --role`; a user-configured `wakeup-command` runs
verbatim and does not receive it.
