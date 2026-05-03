package history

import (
	"context"
	"reflect"
	"testing"

	"gitcode.com/dscli/dscli/internal/prompt"
)

func TestLoadHistory(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		want    []prompt.Message
		wantErr bool
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, gotErr := LoadHistory(context.Background())
			if gotErr != nil {
				if !tt.wantErr {
					t.Errorf("LoadHistory() failed: %v", gotErr)
				}
				return
			}
			if tt.wantErr {
				t.Fatal("LoadHistory() succeeded unexpectedly")
			}
			// TODO: update the condition below to compare got with tt.want.
			if true {
				t.Errorf("LoadHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanupReverse(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		messages []prompt.Message
		want     []prompt.Message
	}{
		{
			"NormalTwo",
			[]prompt.Message{
				{
					Role:       "tool",
					ToolCallID: "01",
				},
				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
					},
				},
			},
			[]prompt.Message{
				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
					},
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},
			},
		},
		{
			"NormalThree",
			[]prompt.Message{
				{
					Role:       "tool",
					ToolCallID: "02",
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},

				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
			},
			[]prompt.Message{
				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},
				{
					Role:       "tool",
					ToolCallID: "02",
				},
			},
		},
		{
			"With1UserMessage",
			[]prompt.Message{
				{
					Role:    "user",
					Content: "content",
				},
				{
					Role:       "tool",
					ToolCallID: "02",
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},

				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
			},
			[]prompt.Message{
				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},
				{
					Role:       "tool",
					ToolCallID: "02",
				},
				{
					Role:    "user",
					Content: "content",
				},
			},
		},

		{
			"With2UserMessage",
			[]prompt.Message{
				{
					Role:    "user",
					Content: "content2",
				},
				{
					Role:       "tool",
					ToolCallID: "02",
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},

				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:    "user",
					Content: "content1",
				},
			},
			[]prompt.Message{
				{
					Role:    "user",
					Content: "content1",
				},

				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:       "tool",
					ToolCallID: "01",
				},
				{
					Role:       "tool",
					ToolCallID: "02",
				},
				{
					Role:    "user",
					Content: "content2",
				},
			},
		},
		{
			"With2UserMessageBad",
			[]prompt.Message{
				{
					Role:    "user",
					Content: "content2",
				},
				{
					Role:       "tool",
					ToolCallID: "02",
				},
				{
					Role:       "tool",
					ToolCallID: "03",
				},

				{
					Role: "assistant",
					ToolCalls: []prompt.ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:    "user",
					Content: "content1",
				},
			},
			[]prompt.Message{
				{
					Role:    "user",
					Content: "content1",
				},

				{
					Role:    "user",
					Content: "content2",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CleanupReverse(tt.messages)
			if !reflect.DeepEqual(tt.want, got) {
				t.Errorf("CleanupReverse() = \n%v, want \n%v", got, tt.want)
			}
		})
	}
}
