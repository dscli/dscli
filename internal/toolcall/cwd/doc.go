// Package cwd implements current-working-directory navigation tools
// (cwd_get, cwd_push, cwd_pop) with a disciplined stack model.
//
// Motivation:
//
// LLM assistants work within a project directory (ProjectRoot).  When they need
// to operate in a different location — a sub-project, a build output directory,
// a test fixture path — every subsequent tool call must either pass absolute
// paths or rely on ambiguous relative-to-root conventions.  Without CWD tools,
// path resolution is brittle and error-prone.
//
// A naive "cd" is insufficient: the assistant needs to return to the original
// context after completing the task.  The stack model matches the natural
// workflow: enter a scope, do work, then return.
//
// Stack Model:
//
//	   PUSH ./sub-project
//	   │  save (CWD=A, ProjectRoot=X)  ──►  stack: [(A, X)]
//	   │  os.Chdir(./sub-project)
//	   │  recompute ProjectRoot (maybe Y)
//	   │
//	   ▼  现在所有文件/Shell 工具都以 sub-project 为基准
//	   ...
//	   POP
//	   │  restore entry: (A, X)
//	   │  os.Chdir(A)
//	   │  context.ProjectRoot = X   ← 恢复 push 前的精确值
//	   ▼
//	   回到原始上下文
//
// Why push+pop Together:
//
// push and pop are two halves of a contract.  push saves state and enters a
// directory; pop restores state and exits.  Separating them (e.g. a "cd" +
// implicit return) would leave the LLM responsible for tracking depth — a
// responsibility better handled by a machine.  The stack is the mechanism that
// makes the contract work.
//
// Why Save ProjectRoot at push (Not Recompute at pop):
//
// pop restores the exact ProjectRoot value saved at push time.  If we
// recomputed ProjectRoot after chdir back, a git branch change or repository
// restructure could yield a different result.  Saving the push-time value
// guarantees the assistant returns to the same project context it left.
//
// Path Normalization:
//
//	filepath.Abs         — resolves relative paths and ".." components
//	filepath.EvalSymlinks — resolves /tmp → /private/tmp on macOS for
//	                        comparison only (storage uses Abs to preserve
//	                        the user's original path semantics)
//
// Edge Cases:
//
//   - Same-directory push: detected via EvalSymlinks comparison, returns
//     "already in ..." without creating a stack entry.
//   - Non-git target: push succeeds with a note.  GetProjectRoot() falls back
//     to CWD when no .git is found.
//   - Stack overflow: maxStackDepth (100) prevents unbounded growth.
//   - Empty pop: returns a message (not an error) — the LLM can safely call
//     pop without checking depth first.
//   - chdir failure after push: the saved stack entry is immediately rolled
//     back, leaving the stack in a consistent state.
//
// Tool Semantics:
//
//	cwd_get  — returns CWD, ProjectRoot, and stack depth (no side effects)
//	cwd_push — (path) pushes state, chdirs, recomputes ProjectRoot
//	cwd_pop  — pops state, restores both CWD and ProjectRoot
//
// Thread Safety:
//
// A sync.Mutex protects dirStack.  While concurrent LLM tool calls are
// unlikely in practice (assistants are single-threaded), the mutex is a
// low-cost guarantee against undefined behavior.
//
// Inter-Package Contract:
//
// This package writes to context.ProjectRoot (internal/context).  No other
// package should mutate ProjectRoot directly — use cwd_push/cwd_pop to
// transition between project scopes.
package cwd
