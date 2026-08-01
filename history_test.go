package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

	msgs, err := prompt.ListHistory(ctx, 0)
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

	msgs, err := prompt.ListHistory(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}

	if len(msgs) != 0 {
		t.Fatalf("expected empty history, got %d messages", len(msgs))
	}
}

func TestHistoryListPagination(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "dscli-test-page-*")
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

	// Real DB isolation: OpenDB caches dbPath at package init, so changing
	// ConfigDir alone would still hit the real ~/.dscli/sqlite.db. Point the
	// DB path at the temp dir explicitly and restore it afterwards.
	origDBPath := sqlite.GetDBPath()
	sqlite.SetDBPath(filepath.Join(tmpDir, "sqlite.db"))
	defer sqlite.SetDBPath(origDBPath)

	ctx := t.Context()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	db.Close(ctx)

	session.ResetSessionID()

	// Insert 5 messages so that with histsize=2 (LIMIT 4) we get 2 pages.
	for i := 1; i <= 5; i++ {
		msg := prompt.Message{
			Role:    "user",
			Content: fmt.Sprintf("message %d", i),
		}
		if err := prompt.SaveMessages(ctx, msg); err != nil {
			t.Fatal(err)
		}
	}

	// NULL reasoning_content (e.g. migrated or hand-inserted rows) must
	// not break scanning. Set the oldest message's to NULL as a guard.
	db, err = sqlite.OpenDB(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE messages SET reasoning_content = NULL WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	db.Close(ctx)

	const histSize = 2 // page size used for this test (LIMIT histSize+2)
	ctx = context.WithValue(ctx, context.HistSizeKey, histSize)

	// Page 1: newest messages, no before-id.
	page1, err := prompt.ListHistory(ctx, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(page1) != histSize+2 {
		t.Fatalf("expected %d messages on page 1 (histsize+2), got %d", histSize+2, len(page1))
	}
	if page1[0].ID >= page1[len(page1)-1].ID {
		t.Fatalf("page 1 should be ascending, got IDs %v", messageIDs(page1))
	}

	// Page 2: keyset continuation from the oldest ID of page 1.
	oldestPage1 := page1[0].ID
	page2, err := prompt.ListHistory(ctx, oldestPage1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2) != 1 {
		t.Fatalf("expected 1 message on page 2, got %d", len(page2))
	}

	// No overlap: page 2 must be strictly older than page 1.
	if page2[0].ID >= oldestPage1 {
		t.Fatalf("page 2 message %d should be older than page 1 oldest %d", page2[0].ID, oldestPage1)
	}

	// The message with NULL reasoning_content (id=1) must survive scanning
	// and appear on page 2; otherwise the NULL guard regressed.
	if page2[0].ID != 1 {
		t.Fatalf("expected the NULL-reasoning message (id=1) on page 2, got %d", page2[0].ID)
	}

	// Union of both pages covers all 5 messages exactly once.
	seen := make(map[int64]bool)
	for _, m := range append(page1, page2...) {
		if seen[m.ID] {
			t.Fatalf("duplicate message ID %d across pages", m.ID)
		}
		seen[m.ID] = true
	}
	if len(seen) != 5 {
		t.Fatalf("expected 5 unique messages across pages, got %d", len(seen))
	}

	// Page 3: beyond the end returns empty.
	page3, err := prompt.ListHistory(ctx, page2[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(page3) != 0 {
		t.Fatalf("expected empty page 3, got %d messages", len(page3))
	}
}

func messageIDs(msgs []*prompt.Message) []int64 {
	ids := make([]int64, len(msgs))
	for i, m := range msgs {
		ids[i] = m.ID
	}
	return ids
}
