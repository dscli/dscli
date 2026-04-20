package toolcall

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
)

func TestToolArgsValue(t *testing.T) {
	ta := func(t *testing.T, toolArgs ToolArgs) (v ToolArgs) {
		b, err := json.Marshal(toolArgs)
		if err != nil {
			t.Fatal(err)
		}
		v = ToolArgs{}
		err = json.Unmarshal(b, &v)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}

	{ // string
		tcs := []struct {
			name         string
			value        string
			defaultValue string
			want         string
		}{
			{"string empty ", "", "a", "a"},
			{"string value", "a", "", "a"},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				if tc.value != "" {
					args["key"] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}
	{ // number - float64
		tcs := []struct {
			name         string
			value        float64
			defaultValue float64
			want         float64
		}{
			{"float64 default", float64(0), float64(1), float64(1)},
			{"float64 default", float64(1), float64(0), float64(1)},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != float64(0) {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // number float32
		tcs := []struct {
			name         string
			value        float32
			defaultValue float32
			want         float32
		}{
			{"float64 default", 0.0, 1.0, 1.0},
			{"float64 default", 1.0, 0.0, 1.0},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != 0.0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // integer - int64
		tcs := []struct {
			name         string
			value        int64
			defaultValue int64
			want         int64
		}{
			{"integer default", int64(0), int64(1), int64(1)},
			{"integer default", int64(1), int64(0), int64(1)},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != int64(0) {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // integer - int
		tcs := []struct {
			name         string
			value        int
			defaultValue int
			want         int
		}{
			{"integer default", int(0), 1, 1},
			{"integer default", int(1), 0, 1},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // integer - int32
		tcs := []struct {
			name         string
			value        int32
			defaultValue int32
			want         int32
		}{
			{"integer default", int32(0), int32(1), int32(1)},
			{"integer default", int32(1), int32(0), int32(1)},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // boolean
		tcs := []struct {
			name         string
			value        bool
			defaultValue bool
			want         bool
		}{
			{"boolean default", false, true, true},
			{"boolean", true, false, true},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != false {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type string
		tcs := []struct {
			name         string
			value        []string
			defaultValue []string
			want         []string
		}{
			{"[]string default", []string{}, []string{"default"}, []string{"default"}},
			{"[]string", []string{"value"}, []string{}, []string{"value"}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type float64
		tcs := []struct {
			name         string
			value        []float64
			defaultValue []float64
			want         []float64
		}{
			{"[]float64 default", []float64{}, []float64{1.0}, []float64{1.0}},
			{"[]float64 default", []float64{2.0}, []float64{}, []float64{2.0}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type float32
		tcs := []struct {
			name         string
			value        []float32
			defaultValue []float32
			want         []float32
		}{
			{"[]float64 default", []float32{}, []float32{1.0}, []float32{1.0}},
			{"[]float64 default", []float32{2.0}, []float32{}, []float32{2.0}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type int64
		tcs := []struct {
			name         string
			value        []int64
			defaultValue []int64
			want         []int64
		}{
			{"[]int64 default", []int64{}, []int64{1}, []int64{1}},
			{"[]int64", []int64{2}, []int64{}, []int64{2}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type int32
		tcs := []struct {
			name         string
			value        []int32
			defaultValue []int32
			want         []int32
		}{
			{"[]int64 default", []int32{}, []int32{1}, []int32{1}},
			{"[]int64", []int32{2}, []int32{}, []int32{2}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type int
		tcs := []struct {
			name         string
			value        []int
			defaultValue []int
			want         []int
		}{
			{"[]int64 default", []int{}, []int{1}, []int{1}},
			{"[]int64", []int{2}, []int{}, []int{2}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array with item type bool
		tcs := []struct {
			name         string
			value        []bool
			defaultValue []bool
			want         []bool
		}{
			{"bool default", []bool{}, []bool{true}, []bool{true}},
			{"[]bool", []bool{true}, []bool{}, []bool{true}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(ta(t, args), "key", tc.defaultValue)
				if !reflect.DeepEqual(actual, tc.want) {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}
}

func TestToolContent(t *testing.T) {
	tcs := []struct {
		name       string
		result     string
		err        error
		suggestion string
		want       string
	}{
		{"all empty", "", nil, "", `{}`},
		{"err empty", "done", nil, "ok", `{
 "result": "done",
 "suggestion": "ok"
}`},
		{"only error", "", fmt.Errorf("all wrong!"), "", `{
 "error": "all wrong!"
}`},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			toolContent := ToolContent{
				Result:     tc.result,
				Suggestion: tc.suggestion,
				Error:      Error(tc.err),
			}
			actual := toolContent.String()
			if actual != tc.want {
				t.Fatal(actual, tc.want)
			}
		})
	}
}

func TestGitCommit(t *testing.T) {
	tcs := []struct {
		name  string
		input string
		want  []string
	}{
		{"git commit one line",
			`{"command":"commit","args":"[\"-m\", \"One line\"]"}`,
			[]string{"-m", "One line"}},
		{"git commit two line",
			`{"command":"commit","args":"[\"-m\", \"One line\\nTwo line\"]"}`,
			[]string{"-m", `One line
Two line`}},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			args := ToolArgs{}
			err := json.Unmarshal([]byte(tc.input), &args)
			if err != nil {
				t.Fatal(err, tc.input)
			}
			command := ToolArgsValue(args, "command", "")
			if command != "commit" {
				t.Fatal(command)
			}
			want := ToolArgsValue(args, "args", []string{})
			if len(want) != 2 {
				t.Fatal(want, args["args"])
			}
			if !reflect.DeepEqual(want, tc.want) {
				t.Fatal(want, tc.want)
			}
		})
	}
}
