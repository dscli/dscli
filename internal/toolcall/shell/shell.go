// Package shell for shell tools
package shell

import "github.com/dscli/dscli/internal/toolcall"

var RegisterTool = toolcall.RegisterTool

type (
	ToolDef   = toolcall.ToolDef
	ToolArgs  = toolcall.ToolArgs
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
