package toolcall

import (
	"encoding/json"
	"testing"
)

// TestRegisterToolAndGetAllTools 测试工具注册和获取
func TestRegisterToolAndGetAllTools(t *testing.T) {
	// 测试获取工具列表
	ctx := t.Context()
	tools := GetAllTools(ctx)
	if len(tools) != 0 {
		t.Error("工具不应存在于工具框架中")
	}
}

func TestFixBrokenJSON(t *testing.T) {
	tests := []struct {
		name   string
		broken string
	}{
		{"empty", ""},
		{"no closing curly brace", `{"path":"main.go", "append":true, "content":"...very...long..."`},
		{"no quote", `{"path":"main.go", "append":true, "content":"...very...long...`},
		{"fake quote", `{"path":"main.go", "append":true, "content":"...very...long\"`},
		{"fake closing curly brace", `{"path":"main.go", "append":true, "content":"...very...long}`},
		{"end with escape", `{"path":"main.go", "append":true, "content":"...very...long\`},
		{"normal broken", `{"path":"main.go", "append":true, "content":"...very...long`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FixBrokenJSON(tt.broken)
			v := map[string]any{}
			err := json.Unmarshal([]byte(got), &v)
			if err != nil {
				t.Fatal(err)
			}
			t.Log(v)
		})
	}
}
