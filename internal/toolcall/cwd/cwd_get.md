# cwd_get tool

Show current working directory and related status.

Returns:

- CWD: absolute path of the current working directory
- ProjectRoot: current project root (used by file tools like read_file / write_file)
- Stack depth: number of pushed directories on the CWD stack

Use `cwd_get` before `cwd_pop` to check stack depth, or anytime to verify the current working context.

This is a read-only system tool — no side effects.
