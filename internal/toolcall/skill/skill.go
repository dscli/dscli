package skill

import "gitcode.com/dscli/dscli/internal/toolcall"

var (
	RegisterTool = toolcall.RegisterTool
)

type (
	ToolArgs  = toolcall.ToolArgs
	ToolDef   = toolcall.ToolDef
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
