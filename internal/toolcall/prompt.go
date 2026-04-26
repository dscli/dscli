package toolcall

import (
	"context"

	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/skills"
)

// LoadPrompts loads the system prompt combined with skill prompts.
func LoadPrompts(ctx context.Context) ([]Message, error) {
	systemPrompt := prompt.GetSystemPrompt(ctx)
	skillPrompt := skills.BuildSkillPrompt(ctx)

	content := systemPrompt
	if skillPrompt != "" {
		content = systemPrompt + "\n\n" + skillPrompt
	}

	return []Message{{
		Role:    "system",
		Content: content,
	}}, nil
}
