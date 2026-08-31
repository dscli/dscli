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
	// "显式零值"用例；slices 经 JSON 往返会坍缩 []any{} 并返回默认值，
	// 该行为由 []string 组的 "known limitation: explicit empty returns default" 用例锁定。
	setKey bool
}

// runToolArgsValueTest 以表驱动方式验证 ToolArgsValue。
// isZero 判断一个 case 的 value 是否等于类型的零值：等于零值的 value
// 模拟"缺少 key"（除非 case 显式 setKey，见 toolArgsCase.setKey）。
func runToolArgsValueTest[T Primitive](t *testing.T, name string, isZero func(T) bool, cases []toolArgsCase[T]) {
	t.Helper()
	// key 是写入与断言使用的固定参数名，仅此 helper 消费。
	const key = "key"
	t.Run(name, func(t *testing.T) {
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				if tc.setKey || !isZero(tc.value) {
					args[key] = tc.value
				}
				actual := ToolArgsValue(marshalToolArgs(t, args), key, tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Errorf("ToolArgsValue(key=%q) = %v, want %v", key, actual, tc.want)
				}
			})
		}
	})
}

func TestToolArgsValue(t *testing.T) {
	runToolArgsValueTest(t, "string", func(v string) bool { return v == "" }, []toolArgsCase[string]{
		{name: "uses default when value is zero", value: "", defaultValue: "a", want: "a", setKey: false},
		{name: "uses value when nonzero", value: "a", defaultValue: "", want: "a", setKey: false},
		{name: "explicit zero value", value: "", defaultValue: "a", want: "", setKey: true},
	})

	runToolArgsValueTest(t, "float64", func(v float64) bool { return v == 0 }, []toolArgsCase[float64]{
		{name: "uses default when value is zero", value: float64(0), defaultValue: float64(1), want: float64(1), setKey: false},
		{name: "uses value when nonzero", value: float64(1), defaultValue: float64(0), want: float64(1), setKey: false},
		{name: "explicit zero value", value: float64(0), defaultValue: float64(1), want: float64(0), setKey: true},
	})

	runToolArgsValueTest(t, "float32", func(v float32) bool { return v == 0.0 }, []toolArgsCase[float32]{
		{name: "uses default when value is zero", value: 0.0, defaultValue: 1.0, want: 1.0, setKey: false},
		{name: "uses value when nonzero", value: 1.0, defaultValue: 0.0, want: 1.0, setKey: false},
		{name: "explicit zero value", value: 0.0, defaultValue: 1.0, want: 0.0, setKey: true},
	})

	runToolArgsValueTest(t, "int64", func(v int64) bool { return v == 0 }, []toolArgsCase[int64]{
		{name: "uses default when value is zero", value: int64(0), defaultValue: int64(1), want: int64(1), setKey: false},
		{name: "uses value when nonzero", value: int64(1), defaultValue: int64(0), want: int64(1), setKey: false},
		{name: "explicit zero value", value: int64(0), defaultValue: int64(1), want: int64(0), setKey: true},
	})

	runToolArgsValueTest(t, "int", func(v int) bool { return v == 0 }, []toolArgsCase[int]{
		{name: "uses default when value is zero", value: 0, defaultValue: 1, want: 1, setKey: false},
		{name: "uses value when nonzero", value: 1, defaultValue: 0, want: 1, setKey: false},
		{name: "explicit zero value", value: 0, defaultValue: 1, want: 0, setKey: true},
	})

	runToolArgsValueTest(t, "int32", func(v int32) bool { return v == 0 }, []toolArgsCase[int32]{
		{name: "uses default when value is zero", value: int32(0), defaultValue: int32(1), want: int32(1), setKey: false},
		{name: "uses value when nonzero", value: int32(1), defaultValue: int32(0), want: int32(1), setKey: false},
		{name: "explicit zero value", value: int32(0), defaultValue: int32(1), want: int32(0), setKey: true},
	})

	runToolArgsValueTest(t, "bool", func(v bool) bool { return !v }, []toolArgsCase[bool]{
		{name: "uses default when value is false", value: false, defaultValue: true, want: true, setKey: false},
		{name: "uses value when true", value: true, defaultValue: false, want: true, setKey: false},
		{name: "explicit zero value", value: false, defaultValue: true, want: false, setKey: true},
	})

	runToolArgsValueTest(t, "[]string", func(v []string) bool { return len(v) == 0 }, []toolArgsCase[[]string]{
		{name: "uses default when empty", value: []string{}, defaultValue: []string{"default"}, want: []string{"default"}, setKey: false},
		{name: "uses value when non-empty", value: []string{"value"}, defaultValue: []string{}, want: []string{"value"}, setKey: false},
		// 已知限制: 显式设置的空 slice 经 JSON 往返后与缺失 key 不可区分，返回默认值。
		{name: "known limitation: explicit empty returns default", value: []string{}, defaultValue: []string{"default"}, want: []string{"default"}, setKey: true},
	})

	runToolArgsValueTest(t, "[]float64", func(v []float64) bool { return len(v) == 0 }, []toolArgsCase[[]float64]{
		{name: "uses default when empty", value: []float64{}, defaultValue: []float64{1.0}, want: []float64{1.0}, setKey: false},
		{name: "uses value when non-empty", value: []float64{2.0}, defaultValue: []float64{}, want: []float64{2.0}, setKey: false},
	})

	runToolArgsValueTest(t, "[]float32", func(v []float32) bool { return len(v) == 0 }, []toolArgsCase[[]float32]{
		{name: "uses default when empty", value: []float32{}, defaultValue: []float32{1.0}, want: []float32{1.0}, setKey: false},
		{name: "uses value when non-empty", value: []float32{2.0}, defaultValue: []float32{}, want: []float32{2.0}, setKey: false},
	})

	runToolArgsValueTest(t, "[]int64", func(v []int64) bool { return len(v) == 0 }, []toolArgsCase[[]int64]{
		{name: "uses default when empty", value: []int64{}, defaultValue: []int64{1}, want: []int64{1}, setKey: false},
		{name: "uses value when non-empty", value: []int64{2}, defaultValue: []int64{}, want: []int64{2}, setKey: false},
	})

	runToolArgsValueTest(t, "[]int32", func(v []int32) bool { return len(v) == 0 }, []toolArgsCase[[]int32]{
		{name: "uses default when empty", value: []int32{}, defaultValue: []int32{1}, want: []int32{1}, setKey: false},
		{name: "uses value when non-empty", value: []int32{2}, defaultValue: []int32{}, want: []int32{2}, setKey: false},
	})

	runToolArgsValueTest(t, "[]int", func(v []int) bool { return len(v) == 0 }, []toolArgsCase[[]int]{
		{name: "uses default when empty", value: []int{}, defaultValue: []int{1}, want: []int{1}, setKey: false},
		{name: "uses value when non-empty", value: []int{2}, defaultValue: []int{}, want: []int{2}, setKey: false},
	})

	runToolArgsValueTest(t, "[]bool", func(v []bool) bool { return len(v) == 0 }, []toolArgsCase[[]bool]{
		{name: "uses default when empty", value: []bool{}, defaultValue: []bool{true}, want: []bool{true}, setKey: false},
		{name: "uses value when non-empty", value: []bool{true}, defaultValue: []bool{}, want: []bool{true}, setKey: false},
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
