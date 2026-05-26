# cwd_pop tool

Restore the previous working directory and ProjectRoot from the CWD stack.

## Behavior

- If stack is empty, returns a message (no error) — stays in current directory
- Otherwise pops the top entry and restores both CWD and ProjectRoot

## Recommended

Use `cwd_get` before `cwd_pop` to verify stack depth. Calling pop on an empty stack is harmless but indicates a logic error in the caller.

This is a system tool — no timeout, no side effects beyond directory state.
