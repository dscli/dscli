// Package shell for shell tools
package shell

import "gitcode.com/dscli/dscli/internal/toolcall"

var (
	RegisterTool   = toolcall.RegisterTool
	TruncateString = toolcall.TruncateString
)

type (
	ToolDef   = toolcall.ToolDef
	ToolArgs  = toolcall.ToolArgs
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}