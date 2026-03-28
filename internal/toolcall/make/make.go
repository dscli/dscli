package make

import "gitcode.com/dscli/dscli/internal/toolcall"

var (
	ShellExec        = toolcall.ShellExec
	RegisterTool     = toolcall.RegisterTool
	TitleLikePattern = toolcall.TitleLikePattern
)

type (
	ToolDef   = toolcall.ToolDef
	ToolArgs  = toolcall.ToolArgs
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
