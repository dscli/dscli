package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
)

func TestHistoryListJSON(t *testing.T) {
	// Create temp directory for test DB
	tmpDir, err := os.MkdirTemp("", "dscli-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override config dir so sqlite.OpenDB uses our temp DB
	origConfigDir := config.ConfigDir
	config.ConfigDir = tmpDir
	defer func() { config.ConfigDir = origConfigDir }()

	// Set project root for session creation
	origProjectRoot := context.ProjectRoot
	context.ProjectRoot = tmpDir
	defer func() { context.ProjectRoot = origProjectRoot }()

	// Open DB to trigger schema initialization
	ctx := t.Context()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Close(ctx)

	// Reset session so it re-resolves to our new DB
	session.ResetSessionID()

	// Insert a test message
	msg := prompt.Message{
		Role:             "user",
		Content:          "Hello, this is a test message",
		ReasoningContent: "thinking...",
	}
	if err := prompt.SaveMessages(ctx, msg); err != nil {
		t.Fatal(err)
	}

	// Insert an assistant message
	assistantMsg := prompt.Message{
		Role:    "assistant",
		Content: "I am an assistant response",
	}
	if err := prompt.SaveMessages(ctx, assistantMsg); err != nil {
		t.Fatal(err)
	}

	// Reset session so the test can re-read
	session.ResetSessionID()

	db, err = sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)

	msgs, err := prompt.ListHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Build JSON output same way as historyListRunE
	type histEntry struct {
		ID               int64             `json:"id"`
		Role             string            `json:"role"`
		Content          string            `json:"content"`
		ReasoningContent string            `json:"reasoning_content,omitempty"`
		ToolCallID       string            `json:"tool_call_id,omitempty"`
		ToolCalls        []prompt.ToolCall `json:"tool_calls,omitempty"`
		CreatedAt        string            `json:"created_at"`
	}
	var result []histEntry
	for _, m := range msgs {
		entry := histEntry{
			ID:               m.ID,
			Role:             m.Role,
			Content:          m.Content,
			ReasoningContent: m.ReasoningContent,
			ToolCallID:       m.ToolCallID,
			ToolCalls:        m.ToolCalls,
			CreatedAt:        m.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		}
		result = append(result, entry)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		t.Fatal(err)
	}

	output := buf.String()

	// Verify it's valid JSON
	var parsed []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output)
	}

	// Check we have 2 messages
	if len(parsed) != 2 {
		t.Fatalf("expected 2 messages, got %d\n%s", len(parsed), output)
	}

	// Verify first message fields
	first := parsed[0]
	if first["role"] != "user" {
		t.Errorf("expected role 'user', got %v", first["role"])
	}
	if first["content"] != "Hello, this is a test message" {
		t.Errorf("unexpected content: %v", first["content"])
	}
	if first["reasoning_content"] != "thinking..." {
		t.Errorf("unexpected reasoning_content: %v", first["reasoning_content"])
	}
	if _, ok := first["id"]; !ok {
		t.Error("missing id field")
	}
	if _, ok := first["created_at"]; !ok {
		t.Error("missing created_at field")
	}

	// Verify second message
	second := parsed[1]
	if second["role"] != "assistant" {
		t.Errorf("expected role 'assistant', got %v", second["role"])
	}

	// Verify omitempty: tool_call_id should be absent for non-tool messages
	if _, ok := first["tool_call_id"]; ok {
		t.Error("tool_call_id should be omitted for user messages")
	}
	if _, ok := second["tool_call_id"]; ok {
		t.Error("tool_call_id should be omitted for assistant messages")
	}
	if _, ok := first["tool_calls"]; ok {
		t.Error("tool_calls should be omitted when empty")
	}
}

func TestHistoryListJSON_Empty(t *testing.T) {
	// Test empty history returns []
	tmpDir, err := os.MkdirTemp("", "dscli-test-empty-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origConfigDir := config.ConfigDir
	config.ConfigDir = tmpDir
	defer func() { config.ConfigDir = origConfigDir }()

	origProjectRoot := context.ProjectRoot
	context.ProjectRoot = tmpDir
	defer func() { context.ProjectRoot = origProjectRoot }()

	ctx := t.Context()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Close(ctx)

	session.ResetSessionID()

	db, err = sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close(ctx)

	msgs, err := prompt.ListHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 0 {
		t.Fatalf("expected empty history, got %d messages", len(msgs))
	}
}
