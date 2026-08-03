# wakeup

Wake up another AI maintainer at a given project, optionally with a message.

- `project` (required): absolute path to the target project directory
- `input` (optional): the message content to send — empty is fine if you've already communicated via mail
- `ainame` (optional): validated against the project's assignment if provided

```
wakeup(input="请 review 我的改动", project="/home/user/project/dscli.el")
wakeup(project="/home/user/project/big-project")  // wake without message
```
