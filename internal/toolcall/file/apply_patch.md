# apply_patch

Apply a unified diff to the working tree. Creates/modifies/deletes files.

Atomic: any conflict fails the whole patch with no partial writes
(git apply semantics, no --reject).

- patch (string, required): unified diff text; or a path to a .patch/.diff
  file (single-line value that names an existing file is read as the patch)
- cwd (string, optional): git repository directory; default project root;
  must stay inside the project root
- check (boolean, optional): true = dry-run (`git apply --check`), no writes
- reverse (boolean, optional): true = reverse-apply (undo, `git apply -R`)

Protected: patches touching sqlite.db or dscli.env are refused.
