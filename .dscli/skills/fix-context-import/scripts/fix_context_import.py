#!/usr/bin/env python3
"""Fix files that import BOTH stdlib "context" AND internal/context (aliased).

The script:
1. Finds Go files importing both stdlib "context" and "internal/context"
2. Removes the stdlib "context" import line
3. Removes the alias from "internal/context" import
4. Replaces all alias.XXX calls with context.XXX
5. Runs gofumpt on modified files
"""

import argparse
import os
import re
import subprocess
import sys
from pathlib import Path

PROJECT_ROOT = None  # set by find_project_root()

MODULE_PATH = "gitcode.com/dscli/dscli/internal/context"

# Exclude the package itself
EXCLUDE_FILES = {
    "internal/context/context.go",
    "internal/context/context_test.go",
}


def find_project_root() -> str:
    """Find git project root."""
    p = Path.cwd()
    while p != p.parent:
        if (p / ".git").exists():
            return str(p)
        p = p.parent
    return str(Path.cwd())


def find_go_files(root: str, targets: list[str] | None = None) -> list[str]:
    """Find all Go files in the project, or specific target files."""
    if targets:
        files = []
        for t in targets:
            p = Path(root) / t
            if p.is_file() and p.suffix == ".go":
                files.append(str(p.relative_to(root)))
            elif p.is_dir():
                for gf in p.rglob("*.go"):
                    files.append(str(gf.relative_to(root)))
        return sorted(set(files))

    files = []
    for gf in Path(root).rglob("*.go"):
        rel = str(gf.relative_to(root))
        if ".git/" in rel:
            continue
        files.append(rel)
    return sorted(files)


def parse_imports(lines: list[str]) -> list[dict]:
    """Extract import info from Go source lines.

    Returns list of dicts with keys:
        line_idx: 0-based index in lines
        text: full import line (stripped)
        path: import path (no quotes, no alias)
        alias: alias name or None
    """
    imports = []
    in_import_block = False
    block_lines = []

    for i, line in enumerate(lines):
        stripped = line.strip()

        # Single import
        m = re.match(r'^import\s+(?:(\w+)\s+)?"([^"]+)"$', stripped)
        if m:
            alias = m.group(1) or None
            path = m.group(2)
            imports.append({
                "line_idx": i,
                "text": stripped,
                "path": path,
                "alias": alias,
            })
            continue

        # Start of grouped import
        if re.match(r'^import\s*\(', stripped):
            in_import_block = True
            continue

        if in_import_block:
            if stripped == ")":
                in_import_block = False
                continue
            if stripped and not stripped.startswith("//"):
                m = re.match(r'^(?:(?:(\w+)\s+)?"([^"]+)")', stripped)
                if m:
                    alias = m.group(1) or None
                    path = m.group(2)
                    imports.append({
                        "line_idx": i,
                        "text": stripped,
                        "path": path,
                        "alias": alias,
                    })

    return imports


def has_both_imports(imports: list[dict]) -> tuple[bool, str | None]:
    """Check if file imports both stdlib 'context' and internal/context.

    Returns (has_both, alias_name) where alias_name is the alias used for
    internal/context, or None.
    """
    has_stdlib = False
    has_internal = False
    internal_alias = None

    for imp in imports:
        if imp["path"] == "context":
            has_stdlib = True
        if imp["path"] == MODULE_PATH:
            has_internal = True
            internal_alias = imp["alias"]

    if has_stdlib and has_internal:
        return True, internal_alias
    return False, None


def fix_file(filepath: str, dry_run: bool = False) -> bool:
    """Fix a single file. Returns True if modified."""
    full_path = os.path.join(PROJECT_ROOT, filepath)

    with open(full_path, "r", encoding="utf-8") as f:
        content = f.read()
        lines = content.split("\n")

    imports = parse_imports(lines)
    has_both, alias = has_both_imports(imports)

    if not has_both:
        return False

    # Find the stdlib "context" import line
    stdlib_line_idx = None
    internal_line_idx = None
    for imp in imports:
        if imp["path"] == "context":
            stdlib_line_idx = imp["line_idx"]
        if imp["path"] == MODULE_PATH:
            internal_line_idx = imp["line_idx"]

    if stdlib_line_idx is None or internal_line_idx is None:
        return False

    new_lines = list(lines)

    # 1. Remove the stdlib "context" import line
    new_lines[stdlib_line_idx] = None  # mark for deletion

    # 2. Remove alias from internal/context line
    old_line = new_lines[internal_line_idx]
    if alias:
        # e.g. 'dsctx "gitcode.com/dscli/dscli/internal/context"'
        #    -> '"gitcode.com/dscli/dscli/internal/context"'
        new_lines[internal_line_idx] = re.sub(
            rf'^\s*{re.escape(alias)}\s+',
            '',
            old_line,
        )

    new_lines = [l for l in new_lines if l is not None]

    # 3. Replace all alias.XXX with context.XXX
    if alias:
        new_content = "\n".join(new_lines)
        # Replace alias.identifiers (not inside strings/comments)
        new_content = re.sub(
            rf'\b{re.escape(alias)}\.',
            'context.',
            new_content,
        )
        new_lines = new_content.split("\n")

    new_content = "\n".join(new_lines)

    if new_content == content:
        return False

    print(f"  {'[DRY-RUN]' if dry_run else 'FIXING'} {filepath}")
    if alias:
        print(f"    alias={alias}, stdlib_line={stdlib_line_idx}, internal_line={internal_line_idx}")

    if not dry_run:
        with open(full_path, "w", encoding="utf-8") as f:
            f.write(new_content)
            # Ensure trailing newline
            if new_content and not new_content.endswith("\n"):
                f.write("\n")

    return True


def main():
    global PROJECT_ROOT
    PROJECT_ROOT = find_project_root()
    os.chdir(PROJECT_ROOT)

    parser = argparse.ArgumentParser(
        description="Fix files that import both stdlib 'context' and internal/context"
    )
    parser.add_argument(
        "--dry-run",
        action="store_true",
        help="Report only, don't modify files",
    )
    parser.add_argument(
        "targets",
        nargs="*",
        help="Specific files or directories to check",
    )
    args = parser.parse_args()

    go_files = find_go_files(PROJECT_ROOT, args.targets or None)

    # Filter out excluded files
    go_files = [f for f in go_files if f not in EXCLUDE_FILES]

    modified = []
    for f in go_files:
        if fix_file(f, dry_run=args.dry_run):
            modified.append(f)

    if not modified:
        print("No dual-import files found.")
        return 0

    print(f"\nFound {len(modified)} file(s) with dual imports.")

    if not args.dry_run and modified:
        print("\nRunning gofumpt on modified files...")
        for f in modified:
            subprocess.run(
                ["gofumpt", "-w", f],
                cwd=PROJECT_ROOT,
                capture_output=True,
            )
        print("Done.")

    return 0


if __name__ == "__main__":
    sys.exit(main())
