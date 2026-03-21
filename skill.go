package main

import (
	"context"
	"fmt"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

// LoadSkills 加载技能到系统提示词中
func LoadSkills(ctx context.Context) (messages []Message, err error) {
	message := Message{
		Role: "system",
	}

	builder := &strings.Builder{}
	skills, err := GetProjectSkills(ctx)
	if err != nil {
		return
	}
	builder.WriteString("The skill list\n")
	builder.WriteString("| ID | Name | Description | Category |")
	for _, skill := range skills {
		fmt.Fprintf(builder, "| %d | %s | %s | %s |\n", skill.ID, skill.Name, skill.Description, skill.Category)
	}
	message.Content = builder.String()
	messages = []Message{message}
	return
}

// safeAsyncRecordUsage 安全的异步记录技能使用
func safeAsyncRecordUsage(skillID int64, projectPath string) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				outfmt.Println("记录技能使用panic:", r)
			}
		}()

		if err := RecordSkillUsage(skillID, projectPath); err != nil {
			outfmt.Println("警告：记录技能使用失败:", err)
		}
	}()
}
