package file

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func Test_handleRipgrep(t *testing.T) {
	tests := []struct {
		name        string
		toolArgs    ToolArgs
		resultLines int
		errs        string
	}{
		{"pattern is required", ToolArgs{}, 1, "pattern is required"},
		{"head_limit=5", ToolArgs{
			"pattern":    "error",
			"head_limit": int64(5),
		}, 5, "<nil>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			args := func()(v ToolArgs) {
				b, err := json.Marshal(tt.toolArgs)
				if err != nil {
					t.Fatal(err)
				}
				v = ToolArgs{}
				err = json.Unmarshal(b, &v)
				if err != nil {
					t.Fatal(err)
				}
				return v
			}()
			v, ok := args["head_limit"]
			t.Log(ok)
			t.Logf("%T", v)
			result, suggestion, err := handleRipgrep(ctx, args)
			errs := fmt.Sprint(err)
			if suggestion != "" {
				t.Fatal(suggestion)
			}

			resultLines := len(strings.Split(result, "\n"))

			if resultLines != tt.resultLines {
				t.Fatal(resultLines, tt.resultLines)
			}

			if errs != tt.errs {
				t.Fatal(errs, tt.errs)
			}
		})
	}
}
