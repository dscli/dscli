package toolcall

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

// Skill 表示一个技能
type Skill struct {
	ID          int64
	Name        string
	Description string
	Content     string
	Category    string
	Priority    int
	IsGlobal    bool
	UsageCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ProjectSkill 表示项目与技能的关联
type ProjectSkill struct {
	ProjectPath string
	SkillID     int64
	IsEnabled   bool
	EnabledAt   time.Time
	LastUsed    sql.NullTime
}

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
