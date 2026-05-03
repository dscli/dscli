package prompt

import (
	"context"
	"reflect"
	"testing"
)

func TestLoadHistory(t *testing.T) {
	tests := []struct {
		name    string // description of this test case
		want    []Message
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
		messages []Message
		want     []Message
	}{
		{
			"NormalTwo",
			[]Message{
				{
					Role:       "tool",
					ToolCallID: "01",
				},
				{
					Role: "assistant",
					ToolCalls: []ToolCall{
						{ID: "01"},
					},
				},
			},
			[]Message{
				{
					Role: "assistant",
					ToolCalls: []ToolCall{
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
			[]Message{
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
					ToolCalls: []ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
			},
			[]Message{
				{
					Role: "assistant",
					ToolCalls: []ToolCall{
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
			[]Message{
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
					ToolCalls: []ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
			},
			[]Message{
				{
					Role: "assistant",
					ToolCalls: []ToolCall{
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
			[]Message{
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
					ToolCalls: []ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:    "user",
					Content: "content1",
				},
			},
			[]Message{
				{
					Role:    "user",
					Content: "content1",
				},

				{
					Role: "assistant",
					ToolCalls: []ToolCall{
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
			[]Message{
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
					ToolCalls: []ToolCall{
						{ID: "01"},
						{ID: "02"},
					},
				},
				{
					Role:    "user",
					Content: "content1",
				},
			},
			[]Message{
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
