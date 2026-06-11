---
name: gh
description: Patterns for invoking the GitHub CLI (gh) from agents. Covers structured output, pagination, repo targeting, search vs list, gh api fallback, release management.
author: Bohr <bohr@dscli.io>
keywords:
- gh
- github
- cli
- release
- upload
- download
- api
- search
- list
---

# Reference

## Interactivity policy

`gh` already does the right thing in non-TTY contexts: it skips the pager,
strips ANSI color, and errors out fast with a helpful message instead of
prompting (e.g. `must provide --title and --body when not running interactively`).
You don't need to defensively set `GH_PAGER` or pass `--no-pager` (no such
flag exists).

## Parsing JSON

Human output from `gh` is column-formatted. If you want structured data:

- Add `--json field1,field2,...` for structured output.
- Run a command with `--json` and **no field list** to print the full set of
  available fields, then pick what you need.
- Use `--jq '<expr>'` for filtering without piping through a separate `jq`.
- Use `--template '<go-template>'` (alongside `--json`) when you want shaped
  text output. Note that `--template`/`-T` collides with a body-template flag
  on a few commands (e.g. `gh pr create -T`, `gh issue create -T`); always
  check `--help` before assuming which one you're hitting.

## Pagination and silent truncation

List commands cap results.

- `gh issue list`, `gh pr list`, `gh search ...`: pass `-L N` (`--limit N`).
  The default is usually 30.
- `gh issue list` / `gh pr list` do not expose aggregate totals like
  `totalCount` via `--json`. If you need a true total, use `gh api graphql`
  to query `totalCount`; otherwise, treat `-L` as the cap for the current call.
- For raw API calls use `gh api --paginate <path>`. Combine with
  `--jq` and (optionally) `--slurp` to assemble one array.

## Repo targeting

`gh` infers the repo from the cwd's git remotes.

Pass `--repo OWNER/REPO` (`-R`) to override the resolved CWD repo.

## Search vs list

- `gh search issues|prs|code|repos|commits|users` uses GitHub's search
  index and accepts the full search syntax (`is:open`, `author:`,
  `label:`, `repo:owner/name`, `in:title`, ...). Pass the entire query as
  one quoted string, the same way you would for `--search`:
  `gh search issues "is:open author:foo repo:cli/cli"`. Prefer it for
  anything cross-repo or filtered by author/label.
- `gh issue list --search "..."` and `gh pr list --search "..."` accept
  the same syntax but are scoped to one repo.

## Fall back to `gh api` for anything `--json` doesn't expose

Sometimes useful data isn't on the typed commands. Examples:

- Review-thread comments on a PR: `gh api repos/{owner}/{repo}/pulls/{n}/comments`
  (the `--comments` flag on `gh pr view` shows issue-level comments only).
- Arbitrary GraphQL: `gh api graphql -f query='...' -F var=value`.
- REST shortcuts: `gh api repos/{owner}/{repo}/...` - note the
  `{owner}/{repo}` placeholder is filled in for you when run from a repo
  with detected remotes; pass them literally if you want determinism.

## Authentication

- `gh auth status` prints the active host(s), user, and which env var (if
  any) is being honored.
- `gh auth status --json` is supported.

## Other notes

- `gh pr checkout <n>` switches branches. Use `gh pr diff <n>` or
  `gh pr view <n>` if you only need to read.
- `NO_COLOR`, `CLICOLOR_FORCE`, and `GH_FORCE_TTY` are honored. Set
  `GH_FORCE_TTY=1` if you want TTY-style output (colors, tables, the
  full interactive experience).

## Release management

Release is a two-phase workflow: **create** the release, then **upload** assets.

### Create a release

Use `gh release create <tag>`:

- `--generate-notes` --- auto-generate changelog from merged PRs and commits.
- `--notes-file <file>` --- supply custom release notes from a file.
- `--verify-tag` --- refuse to proceed unless the tag exists on the remote.
- `--title <title>` --- optional; defaults to the tag name.
- Set `--draft` or `--prerelease` as needed.
- If the tag already exists on the remote, `gh release create` fails. Create a release for an existing tag with `gh release create <tag> --notes "..."`.

Examples:

  gh release create v1.0.0 --generate-notes --verify-tag
  gh release create v1.0.0 --notes-file .changes/v1.0.0.md --title "v1.0.0"

### Upload assets

After the release exists, upload binaries:

  gh release upload <tag> <file1> [<file2> ...] [--clobber]

- `--clobber` --- overwrite an existing asset of the same name (safe to always pass).
- Upload from a directory with globs: `gh release upload <tag> _release/*`.
- Verify with `gh release view <tag> --json assets --jq '.assets[] | {name, size, state}'`.

Example (CI-style cross-compile + upload):

  gh release upload v1.0.0 \
    _release/myapp-linux-amd64 \
    _release/myapp-linux-arm64 \
    _release/myapp-darwin-amd64 \
    _release/myapp-darwin-arm64 \
    _release/myapp-windows-amd64.exe \
    _release/myapp-windows-arm64.exe \
    --clobber

### List and download

- `gh release list` --- list releases (`-L N` for limit, `--json tagName,name`).
- `gh release download <tag>` --- download all assets to cwd.
- `gh release download <tag> -p '<pattern>'` --- download files matching a glob pattern.
