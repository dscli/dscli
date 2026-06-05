package alltools

import (
	"context"
	"testing"

	"github.com/dscli/dscli/internal/toolcall"
)

func TestGetAllTools(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		checker func(toolcall.Tool) bool
	}{
		{"in strict", func(tool toolcall.Tool) bool {
			return tool.Function.Strict
		}},
		{"no additional property", func(tool toolcall.Tool) bool {
			if additionalProperties, ok := tool.Function.Parameters["additionalProperties"]; ok {
				return !additionalProperties.(bool)
			}
			return false
		}},
		{"not too large", func(tool toolcall.Tool) bool {
			return tool.GetTokens() <= 1600 // around 1.5K
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetAllTools(context.Background())
			for _, tool := range got {
				if !tt.checker(tool) {
					t.Fatal(tool.Function.Strict, tool.Function.Name, tool.Function.Description)
				}
			}
		})
	}
}
