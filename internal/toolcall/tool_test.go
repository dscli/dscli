package toolcall

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
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

func TestToolCallsUnmarshal(t *testing.T) {
	data := []byte(`[{"id":"call_00_hwUc2FNhUQ45gf3kCdq299Cu",` +
		`"type":"function","function":{"name":"git","arguments":"{\"command\": ` +
		`\"commit\", \"args\": [\"-m\",\"fix(git): improve args/arguments ` +
		`parameter handling logic\"]}"}}]`)
	tcs := []prompt.ToolCall{}

	err := json.Unmarshal(data, &tcs)
	if err != nil {
		t.Fatal(err)
	}

	if len(tcs) == 0 {
		t.Fatal(tcs)
	}
	tc := tcs[0]
	arguments := tc.Function.Arguments
	if len(arguments) == 0 {
		t.Fatal(arguments)
	}

	toolArgs := ToolArgs{}
	err = json.Unmarshal([]byte(arguments), &toolArgs)
	if err != nil {
		t.Fatal(err)
	}

	command := ToolArgsValue(toolArgs, "command", "")
	if command == "" {
		t.Fatal()
	}

	args := ToolArgsValue(toolArgs, "args", []string{})
	if len(args) == 0 {
		t.Fatal(args, toolArgs)
	}
}

func TestToolCallIndexPrefix(t *testing.T) {
	tests := []struct {
		name  string
		index int
		total int
		want  string
	}{
		{"no index", 0, 0, ""},
		{"single call no index", 1, 1, ""},
		{"indexed", 1, 2, "[1/2] "},
		{"second of three", 2, 3, "[2/3] "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := toolCallIndexPrefix(tt.index, tt.total); got != tt.want {
				t.Errorf("toolCallIndexPrefix(%d, %d) = %q, want %q",
					tt.index, tt.total, got, tt.want)
			}
		})
	}
}

func TestHandleToolCallsDisplay(t *testing.T) {
	// 快照并恢复注册表，避免污染其他测试
	toolRegistryRWMutex.RLock()
	snapshot := maps.Clone(toolRegistry)
	toolRegistryRWMutex.RUnlock()
	t.Cleanup(func() {
		toolRegistryRWMutex.Lock()
		toolRegistry = snapshot
		toolRegistryRWMutex.Unlock()
	})

	if err := RegisterTool(ToolDef{
		Name:        "test_display",
		DisplayName: "TestDisplay",
		Description: "display test tool",
		Handler: func(_ context.Context, _ ToolArgs) (result, warning string, err error) {
			return "ok", "", nil
		},
	}); err != nil {
		t.Fatal(err)
	}

	// 捕获输出，结束后恢复默认写入器
	run := func(t *testing.T, tcs []prompt.ToolCall) string {
		t.Helper()
		var buf bytes.Buffer
		outfmt.SetOutputWriter(&buf)
		t.Cleanup(func() { outfmt.SetOutputWriter(os.Stdout) })
		inputs := HandleToolCalls(t.Context(), tcs)
		if len(inputs) != len(tcs) {
			t.Fatalf("HandleToolCalls returned %d inputs, want %d", len(inputs), len(tcs))
		}
		return buf.String()
	}

	t.Run("batch shows summary and index", func(t *testing.T) {
		got := run(t, []prompt.ToolCall{
			{ID: "call_1", Type: "function", Function: prompt.ToolCallFunction{Name: "test_display", Arguments: `{}`}},
			{ID: "call_2", Type: "function", Function: prompt.ToolCallFunction{Name: "test_display", Arguments: `{}`}},
		})
		wantLines := []string{
			"📋 本轮共 2 个工具调用",
			"🔄 [1/2] 正在执行 TestDisplay...",
			"✅ [1/2] TestDisplay 执行成功",
			"🔄 [2/2] 正在执行 TestDisplay...",
			"✅ [2/2] TestDisplay 执行成功",
		}
		gotLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
		if !slices.Equal(gotLines, wantLines) {
			t.Errorf("output lines = %q, want %q", gotLines, wantLines)
		}
	})

	t.Run("single call no summary no index", func(t *testing.T) {
		got := run(t, []prompt.ToolCall{
			{ID: "call_1", Type: "function", Function: prompt.ToolCallFunction{Name: "test_display", Arguments: `{}`}},
		})
		for _, unwanted := range []string{"📋", "[1/1]"} {
			if strings.Contains(got, unwanted) {
				t.Errorf("output should not contain %q, got:\n%s", unwanted, got)
			}
		}
		if !strings.Contains(got, "🔄 正在执行 TestDisplay...") {
			t.Errorf("output missing start line, got:\n%s", got)
		}
	})

	t.Run("empty calls no output", func(t *testing.T) {
		if got := run(t, nil); got != "" {
			t.Errorf("expected no output, got:\n%s", got)
		}
	})
}
