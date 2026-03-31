package toolcall

import (
	"fmt"
	"reflect"
	"testing"
)

func TestToolArgsValue(t *testing.T) {
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
				actual := ToolArgsValue(args, "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}
	{ // float64
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
				actual := ToolArgsValue(args, "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // integer
		tcs := []struct {
			name         string
			value        int64
			defaultValue int64
			want         int64
		}{
			{"float64 default", int64(0), int64(1), int64(1)},
			{"float64 default", int64(1), int64(0), int64(1)},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != int64(0) {
					args[key] = tc.value
				}
				actual := ToolArgsValue(args, "key", tc.defaultValue)
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
			{"float64 default", false, true, true},
			{"float64 default", true, false, true},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if tc.value != false {
					args[key] = tc.value
				}
				actual := ToolArgsValue(args, "key", tc.defaultValue)
				if actual != tc.want {
					t.Fatal(actual, tc.want)
				}
			})
		}
	}

	{ // array
		tcs := []struct {
			name         string
			value        []string
			defaultValue []string
			want         []string
		}{
			{"float64 default", []string{}, []string{"default"}, []string{"default"}},
			{"float64 default", []string{"value"}, []string{}, []string{"value"}},
		}

		for _, tc := range tcs {
			t.Run(tc.name, func(t *testing.T) {
				args := ToolArgs{}
				key := "key"
				if len(tc.value) != 0 {
					args[key] = tc.value
				}
				actual := ToolArgsValue(args, "key", tc.defaultValue)
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
