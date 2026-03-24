package toolcall_test

import (
	"strings"
	"testing"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

func TestLoadPrompts(t *testing.T) {
	tests := []struct {
		name        string
		modelID     int64
		contains    string
		notcontains string
	}{
		{
			"deepseek-chat",
			context.DeepseekChat,
			"专业编程助手",
			"system_prompt",
		},
		{
			"deepseek-reasoner",
			context.DeepseekReasoner,
			"编程领域一个深入思考者",
			"system_prompt",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			ctx = context.WithValue(ctx, context.CurrentDomainIDKey, int64(0))
			ctx = context.WithValue(ctx, context.CurrentModelIDKey, tt.modelID)
			got, err := toolcall.LoadPrompts(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1 {
				t.Fatal(got)
			}
			content := got[0].Content
			if !strings.Contains(content, tt.contains) {
				t.Fatal(content, tt.contains)
			}
			if strings.Contains(content, tt.notcontains){
				t.Fatal(content, tt.notcontains)
			}
		})
	}
}
