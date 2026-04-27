// Package git provides git tool for LLM to use.
// The implementation is a piece, while the main purpose is to provide
// the git help system, which is like a git skill system.
package git

import (
	"gitcode.com/dscli/dscli/internal/toolcall"
)

var (
	RegisterTool = toolcall.RegisterTool
	ShellExec    = toolcall.ShellExec
)

type (
	ToolDef   = toolcall.ToolDef
	ToolArgs  = toolcall.ToolArgs
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
