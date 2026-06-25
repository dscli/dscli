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

func TestCompressHistory(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     []Message
	}{
		{
			name: "empty",
		},
		{
			name: "single user message",
			messages: []Message{
				{Role: "user", Content: "hello"},
			},
			want: []Message{
				{Role: "user", Content: "hello"},
			},
		},
		{
			name: "filters out assistant with tool_calls and tool messages",
			messages: []Message{
				{Role: "user", Content: "edit file"},
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "01"}}},
				{Role: "tool", ToolCallID: "01"},
				{Role: "assistant", Content: "done"},
			},
			want: []Message{
				{Role: "user", Content: "edit file"},
				{Role: "assistant", Content: "done"},
			},
		},
		{
			name: "multiple turns with tool calls",
			messages: []Message{
				{Role: "user", Content: "create file"},
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "01"}}},
				{Role: "tool", ToolCallID: "01", Content: "ok"},
				{Role: "assistant", Content: "created"},
				{Role: "user", Content: "add tests"},
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "02"}}},
				{Role: "tool", ToolCallID: "02", Content: "ok"},
				{Role: "assistant", Content: "tests added"},
			},
			want: []Message{
				{Role: "user", Content: "create file"},
				{Role: "assistant", Content: "created"},
				{Role: "user", Content: "add tests"},
				{Role: "assistant", Content: "tests added"},
			},
		},
		{
			name: "keeps assistant without tool_calls",
			messages: []Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
			},
			want: []Message{
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "hi"},
			},
		},
		{
			name: "multiple tool calls per turn",
			messages: []Message{
				{Role: "user", Content: "do multiple"},
				{Role: "assistant", ToolCalls: []ToolCall{{ID: "01"}, {ID: "02"}}},
				{Role: "tool", ToolCallID: "01"},
				{Role: "tool", ToolCallID: "02"},
				{Role: "assistant", Content: "all done"},
			},
			want: []Message{
				{Role: "user", Content: "do multiple"},
				{Role: "assistant", Content: "all done"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compressHistory(tt.messages)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("compressHistory() = %v, want %v", got, tt.want)
			}
		})
	}
}
