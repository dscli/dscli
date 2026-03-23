// Package shell for shell tools like shell and python
package shell

import "gitcode.com/dscli/dscli/internal/toolcall"

var (
	RegisterTool       = toolcall.RegisterTool
	ContentLikePattern = toolcall.ContentLikePattern
	TitleLikePattern   = toolcall.TitleLikePattern
	TruncateString     = toolcall.TruncateString
	RunShell           = toolcall.RunShell
)

type (
	ToolDef  = toolcall.ToolDef
	ToolArgs = toolcall.ToolArgs
)

func ToolArgsValue[T any](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
