package misc

import "gitcode.com/dscli/dscli/internal/toolcall"

var (
	GetSkillByName        = toolcall.GetSkillByName
	RegisterTool          = toolcall.RegisterTool
	TitleLikePattern      = toolcall.TitleLikePattern
	SafeAsyncRecordUsage  = toolcall.SafeAsyncRecordUsage
	NewSystemPromptConfig = toolcall.NewSystemPromptConfig
)

type (
	ToolArgs = toolcall.ToolArgs
	ToolDef  = toolcall.ToolDef
)

func ToolArgsValue[T any](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}
