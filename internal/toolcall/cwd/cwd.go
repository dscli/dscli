// Package cwd implements current-working-directory tools (cwd_get, cwd_push, cwd_pop)
package cwd

import (
	"sync"

	"github.com/dscli/dscli/internal/toolcall"
)

var RegisterTool = toolcall.RegisterTool

type (
	ToolDef   = toolcall.ToolDef
	ToolArgs  = toolcall.ToolArgs
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}

// dirEntry saves one level of CWD/ProjectRoot state on the stack.
type dirEntry struct {
	CWD         string
	ProjectRoot string
}

const maxStackDepth = 100

var (
	dirStack   []dirEntry
	dirStackMu sync.Mutex
)
