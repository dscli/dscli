package misc

import "gitcode.com/dscli/dscli/internal/toolcall"

var (
	GetSkillByName       = toolcall.GetSkillByName
	RegisterTool         = toolcall.RegisterTool
	TitleLikePattern     = toolcall.TitleLikePattern
	SafeAsyncRecordUsage = toolcall.SafeAsyncRecordUsage
)

type (
	ToolArgs  = toolcall.ToolArgs
	ToolDef   = toolcall.ToolDef
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
