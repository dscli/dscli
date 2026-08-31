package toolcall

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

// marshalToolArgs 往返 JSON 序列化 ToolArgs（模拟工具参数经 API 传输）。
func marshalToolArgs(t *testing.T, toolArgs ToolArgs) ToolArgs {
	t.Helper()
	b, err := json.Marshal(toolArgs)
	if err != nil {
		t.Fatal(err)
	}
	v := ToolArgs{}
	if err = json.Unmarshal(b, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

type toolArgsCase[T Primitive] struct {
	name         string
	value        T
	defaultValue T
	want         T
	// setKey 为 true 时无条件写入 key（即使 value 为零值），用于覆盖
	// "显式零值"用例；slices 不适用（JSON 往返会坍缩 []any{} 并返回默认值）。
	setKey bool
}

// runToolArgsValueTest 以表驱动方式验证 ToolArgsValue。
// isAbsent 判定一个 case 的 value 是否等于类型的零值：等于零值的 value
// 模拟"缺少 key"（除非 case 显式 setKey，见 toolArgsCase.setKey）。
func runToolArgsValueTest[T Primitive](t *testing.T, name string, isAbsent func(T) bool, cases []toolArgsCase[T]) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				if tc.setKey || !isAbsent(tc.value) {
					args["key"] = tc.value
				}
				actual := ToolArgsValue(marshalToolArgs(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatalf("ToolArgsValue(key=%q) = %v, want %v", "key", actual, tc.want)
				}
			})
		}
	})
}

func TestToolArgsValue(t *testing.T) {
	runToolArgsValueTest(t, "string", func(v string) bool { return v == "" }, []toolArgsCase[string]{
		{"uses default when value is zero", "", "a", "a", false},
		{"uses value when nonzero", "a", "", "a", false},
		{"explicit zero value", "", "a", "", true},
	})

	runToolArgsValueTest(t, "float64", func(v float64) bool { return v == 0 }, []toolArgsCase[float64]{
		{"uses default when value is zero", float64(0), float64(1), float64(1), false},
		{"uses value when nonzero", float64(1), float64(0), float64(1), false},
		{"explicit zero value", float64(0), float64(1), float64(0), true},
	})

	runToolArgsValueTest(t, "float32", func(v float32) bool { return v == 0.0 }, []toolArgsCase[float32]{
		{"uses default when value is zero", 0.0, 1.0, 1.0, false},
		{"uses value when nonzero", 1.0, 0.0, 1.0, false},
		{"explicit zero value", 0.0, 1.0, 0.0, true},
	})

	runToolArgsValueTest(t, "int64", func(v int64) bool { return v == 0 }, []toolArgsCase[int64]{
		{"uses default when value is zero", int64(0), int64(1), int64(1), false},
		{"uses value when nonzero", int64(1), int64(0), int64(1), false},
		{"explicit zero value", int64(0), int64(1), int64(0), true},
	})

	runToolArgsValueTest(t, "int", func(v int) bool { return v == 0 }, []toolArgsCase[int]{
		{"uses default when value is zero", int(0), 1, 1, false},
		{"uses value when nonzero", int(1), 0, 1, false},
		{"explicit zero value", int(0), 1, 0, true},
	})

	runToolArgsValueTest(t, "int32", func(v int32) bool { return v == 0 }, []toolArgsCase[int32]{
		{"uses default when value is zero", int32(0), int32(1), int32(1), false},
		{"uses value when nonzero", int32(1), int32(0), int32(1), false},
		{"explicit zero value", int32(0), int32(1), int32(0), true},
	})

	runToolArgsValueTest(t, "bool", func(v bool) bool { return !v }, []toolArgsCase[bool]{
		{"uses default when value is zero", false, true, true, false},
		{"uses value when nonzero", true, false, true, false},
		{"explicit zero value", false, true, false, true},
	})

	runToolArgsValueTest(t, "[]string", func(v []string) bool { return len(v) == 0 }, []toolArgsCase[[]string]{
		{"uses default when empty", []string{}, []string{"default"}, []string{"default"}, false},
		{"uses value when non-empty", []string{"value"}, []string{}, []string{"value"}, false},
	})

	runToolArgsValueTest(t, "[]float64", func(v []float64) bool { return len(v) == 0 }, []toolArgsCase[[]float64]{
		{"uses default when empty", []float64{}, []float64{1.0}, []float64{1.0}, false},
		{"uses value when non-empty", []float64{2.0}, []float64{}, []float64{2.0}, false},
	})

	runToolArgsValueTest(t, "[]float32", func(v []float32) bool { return len(v) == 0 }, []toolArgsCase[[]float32]{
		{"uses default when empty", []float32{}, []float32{1.0}, []float32{1.0}, false},
		{"uses value when non-empty", []float32{2.0}, []float32{}, []float32{2.0}, false},
	})

	runToolArgsValueTest(t, "[]int64", func(v []int64) bool { return len(v) == 0 }, []toolArgsCase[[]int64]{
		{"uses default when empty", []int64{}, []int64{1}, []int64{1}, false},
		{"uses value when non-empty", []int64{2}, []int64{}, []int64{2}, false},
	})

	runToolArgsValueTest(t, "[]int32", func(v []int32) bool { return len(v) == 0 }, []toolArgsCase[[]int32]{
		{"uses default when empty", []int32{}, []int32{1}, []int32{1}, false},
		{"uses value when non-empty", []int32{2}, []int32{}, []int32{2}, false},
	})

	runToolArgsValueTest(t, "[]int", func(v []int) bool { return len(v) == 0 }, []toolArgsCase[[]int]{
		{"uses default when empty", []int{}, []int{1}, []int{1}, false},
		{"uses value when non-empty", []int{2}, []int{}, []int{2}, false},
	})

	runToolArgsValueTest(t, "[]bool", func(v []bool) bool { return len(v) == 0 }, []toolArgsCase[[]bool]{
		{"uses default when empty", []bool{}, []bool{true}, []bool{true}, false},
		{"uses value when non-empty", []bool{true}, []bool{}, []bool{true}, false},
	})
}

func TestToolContent(t *testing.T) {
	tcs := []struct {
		name    string
		index   int
		tool    string
		result  string
		err     error
		warning string
		want    string
	}{
		{"all empty", 0, "", "", nil, "", ``},
		{"with warning", 0, "", "done", nil, "ok", "### Result\ndone\n\n### Warning\nok\n"},
		{"only error", 0, "", "", fmt.Errorf("all wrong!"), "", "### Error\nall wrong!\n"},
		{"with index", 1, "read_file", "done", nil, "", "Tool result 1 (read_file):\n### Result\ndone\n"},
		{"with index and warning", 2, "shell", "", nil, "warning msg", "Tool result 2 (shell):\n### Warning\nwarning msg\n"},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			toolContent := ToolContent{
				Index:    tc.index,
				ToolName: tc.tool,
				Result:   tc.result,
				Warning:  tc.warning,
				Error:    Error(tc.err),
			}
			actual := toolContent.String()
			if actual != tc.want {
				t.Fatalf("got %q, want %q", actual, tc.want)
			}
		})
	}
}

func TestToolArgsValue_WithJsonStringArray(t *testing.T) {
	tcs := []struct {
		name     string
		input    string // 完整的JSON输入
		key      string
		expected any
	}{
		{
			name:     "normal array",
			input:    `{"args": ["-m", "msg"]}`,
			key:      "args",
			expected: []string{"-m", "msg"},
		},
		{
			name:     "json string array",
			input:    `{"args": "[\"-m\", \"msg\"]"}`,
			key:      "args",
			expected: []string{"-m", "msg"},
		},
		{
			name:     "json string array with spaces",
			input:    `{"args": "[ \"-m\", \"msg\" ]"}`,
			key:      "args",
			expected: []string{"-m", "msg"},
		},
		{
			name:     "invalid json string",
			input:    `{"args": "[\"-m\", \"msg\""}`,
			key:      "args",
			expected: `["-m", "msg"`,
		},
		{
			name:     "not a json array string",
			input:    `{"message": "hello [\"world\"]"}`,
			key:      "message",
			expected: `hello ["world"]`,
		},
		{
			name:     "empty json array",
			input:    `{"args": "[]"}`,
			key:      "args",
			expected: []string{},
		},
		{
			name:     "json string with newline",
			input:    `{"args": "[\"-m\", \"One line\\nTwo line\"]"}`,
			key:      "args",
			expected: []string{"-m", "One line\nTwo line"},
		},
		{
			name:     "json number array as string",
			input:    `{"numbers": "[1, 2, 3]"}`,
			key:      "numbers",
			expected: []float64{1, 2, 3},
		},
		{
			name:     "json boolean array as string",
			input:    `{"flags": "[true, false, true]"}`,
			key:      "flags",
			expected: []bool{true, false, true},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			args := ToolArgs{}
			err := json.Unmarshal([]byte(tc.input), &args)
			if err != nil {
				t.Fatal(err, tc.input)
			}

			// 根据期望类型调用不同的ToolArgsValue
			switch expected := tc.expected.(type) {
			case []string:
				got := ToolArgsValue(args, tc.key, []string{})
				if !reflect.DeepEqual(got, expected) {
					t.Errorf("ToolArgsValue(%q) = %v, want %v", tc.key, got, expected)
				}
			case string:
				got := ToolArgsValue(args, tc.key, "")
				if got != expected {
					t.Errorf("ToolArgsValue(%q) = %q, want %q", tc.key, got, expected)
				}
			case []float64:
				got := ToolArgsValue(args, tc.key, []float64{})
				if !reflect.DeepEqual(got, expected) {
					t.Errorf("ToolArgsValue(%q) = %v, want %v", tc.key, got, expected)
				}
			case []bool:
				got := ToolArgsValue(args, tc.key, []bool{})
				if !reflect.DeepEqual(got, expected) {
					t.Errorf("ToolArgsValue(%q) = %v, want %v", tc.key, got, expected)
				}
			}
		})
	}
}
