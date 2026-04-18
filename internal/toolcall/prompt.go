package toolcall

import (
	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/prompt"
)


// LoadPrompts 加载提示词
func LoadPrompts(ctx context.Context) ([]Message, error) {
	return []Message{{
		Role:    "system",
		Content: prompt.GetSystemPrompt(ctx),
	}}, nil
}


