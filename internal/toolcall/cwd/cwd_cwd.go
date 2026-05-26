package cwd

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"gitcode.com/dscli/dscli/internal/context"
)

//go:embed cwd_get.md
var cwd_get_md string

//go:embed cwd_push.md
var cwd_push_md string

//go:embed cwd_pop.md
var cwd_pop_md string

func init() {
	// cwd_get — read-only status
	RegisterTool(ToolDef{
		Name:        "cwd_get",
		Description: cwd_get_md,
		Category:    "system",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Handler: handleCWDGet,
	})

	// cwd_push — push current dir + chdir to target
	RegisterTool(ToolDef{
		Name:        "cwd_push",
		Description: cwd_push_md,
		Category:    "system",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Target directory path (relative or absolute). Will be resolved to absolute path via filepath.Abs.",
				},
			},
			"required":             []string{"path"},
			"additionalProperties": false,
		},
		Handler: handleCWDPush,
	})

	// cwd_pop — restore previous dir from stack
	RegisterTool(ToolDef{
		Name:        "cwd_pop",
		Description: cwd_pop_md,
		Category:    "system",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Handler: handleCWDPop,
	})
}

// handleCWDGet returns current CWD, ProjectRoot and stack depth.
func handleCWDGet(_ context.Context, _ ToolArgs) (result, warning string, err error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("cannot get current directory: %w", err)
	}

	dirStackMu.Lock()
	depth := len(dirStack)
	dirStackMu.Unlock()

	result = fmt.Sprintf("CWD: %s\nProjectRoot: %s\nStack depth: %d", cwd, context.ProjectRoot, depth)
	return result, "", nil
}

// handleCWDPush pushes current directory onto the CWD stack and changes to target.
func handleCWDPush(_ context.Context, args ToolArgs) (result, warning string, err error) {
	path := ToolArgsValue(args, "path", "")
	if path == "" {
		return "", "", fmt.Errorf("path is required")
	}

	// Resolve to absolute path
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", "", fmt.Errorf("cannot resolve path %q: %w", path, err)
	}

	// Verify target
	info, err := os.Stat(absPath)
	if err != nil {
		return "", "", fmt.Errorf("cannot access %q: %w", absPath, err)
	}
	if !info.IsDir() {
		return "", "", fmt.Errorf("%q is not a directory", absPath)
	}

	// Get current CWD for comparison and saving
	currentCWD, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("cannot get current directory: %w", err)
	}

	// Compare after resolving symlinks (e.g. /tmp → /private/tmp)
	cmpTarget, _ := filepath.EvalSymlinks(absPath)
	cmpCurrent, _ := filepath.EvalSymlinks(currentCWD)
	if cmpTarget == cmpCurrent {
		return "already in " + absPath, "", nil
	}

	// Check stack depth
	dirStackMu.Lock()
	if len(dirStack) >= maxStackDepth {
		dirStackMu.Unlock()
		return "", "", fmt.Errorf("directory stack overflow (max %d)", maxStackDepth)
	}

	// Save current state
	dirStack = append(dirStack, dirEntry{
		CWD:         currentCWD,
		ProjectRoot: context.ProjectRoot,
	})
	depth := len(dirStack)
	dirStackMu.Unlock()

	// Change directory
	if err := os.Chdir(absPath); err != nil {
		// Rollback
		dirStackMu.Lock()
		dirStack = dirStack[:len(dirStack)-1]
		dirStackMu.Unlock()
		return "", "", fmt.Errorf("cannot chdir to %q: %w", absPath, err)
	}

	// Recompute ProjectRoot from new CWD
	context.ProjectRoot = context.GetProjectRoot()

	// Detect non-git directory: GetProjectRoot falls back to CWD when no .git found
	isGit := true
	gitPath := filepath.Join(context.ProjectRoot, ".git")
	if _, err := os.Stat(gitPath); os.IsNotExist(err) {
		isGit = false
	}

	result = fmt.Sprintf("Pushed to %s (stack depth: %d)", absPath, depth)
	if !isGit {
		result += "\nNote: not a git repository, using directory as project root"
	}
	return result, "", nil
}

// handleCWDPop restores the previous directory from the CWD stack.
func handleCWDPop(_ context.Context, _ ToolArgs) (result, warning string, err error) {
	dirStackMu.Lock()
	if len(dirStack) == 0 {
		dirStackMu.Unlock()
		cwd, _ := os.Getwd()
		return fmt.Sprintf("Stack is empty, already at initial directory: %s", cwd), "", nil
	}

	// Pop top entry
	entry := dirStack[len(dirStack)-1]
	dirStack = dirStack[:len(dirStack)-1]
	depth := len(dirStack)
	dirStackMu.Unlock()

	// Restore directory
	if err := os.Chdir(entry.CWD); err != nil {
		return "", "", fmt.Errorf("cannot restore CWD to %q: %w", entry.CWD, err)
	}

	// Restore ProjectRoot
	context.ProjectRoot = entry.ProjectRoot

	result = fmt.Sprintf("Popped to %s (stack depth: %d)", entry.CWD, depth)
	if entry.ProjectRoot != "" {
		result += fmt.Sprintf("\nProjectRoot: %s", entry.ProjectRoot)
	}
	return result, "", nil
}