# cwd_push tool

Push current directory onto the CWD stack and change to the target directory.

## Parameters

- `path` (required): target directory — absolute path or relative to current CWD

## Behavior

1. Resolves the path to an absolute path via `filepath.Abs`
2. Validates the target exists and is a directory (`os.Stat`)
3. If target equals current CWD, returns early with "already in ..."
4. If stack is at max depth (100), returns error
5. Saves current CWD + ProjectRoot onto the stack
6. Changes working directory (`os.Chdir`) and recomputes `context.ProjectRoot`

## Note

After switching, tools that depend on `context.ProjectRoot` (read_file, write_file, sql, flycheck, etc.) resolve paths relative to the new project root. If the new project lacks `sqlite.db` or `dscli.env`, those tools may be unavailable.

Push to a non-git directory works — the directory itself becomes the project root.
