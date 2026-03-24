package toolcall

import (
	"gitcode.com/dscli/dscli/internal/context"
)

// GetSystemPrompt 获取系统提示词
func GetSystemPrompt(ctx context.Context) string {
	// 获取当前项目的领域ID
	domainID := GetCurrentDomainID(ctx)

	// 获取当前模型ID
	modelID := GetCurrentModelID(ctx)

	// 获取系统提示词配置
	config := GetSystemPromptConfig(ctx)

	// 使用段落管理器渲染系统提示词
	sm := &SegmentManager{}
	prompt, err := sm.RenderSystemPrompt(ctx, modelID, domainID, config)
	if err != nil || prompt == "" {
		return ""
	}
	return prompt
}

// LoadPrompts 加载提示词
func LoadPrompts(ctx context.Context) ([]Message, error) {
	return LoadEnhancedPrompts(ctx)
}

// GetCurrentDomainID 获取当前项目的领域ID
func GetCurrentDomainID(ctx context.Context) int64 {
	return context.ContextValue(ctx, context.CurrentDomainIDKey, int64(0))
}

// GetCurrentModelID 获取当前模型ID
func GetCurrentModelID(ctx context.Context) int64 {
	return context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
}
