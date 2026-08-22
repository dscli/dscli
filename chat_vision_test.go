package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	dcontext "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/spf13/cobra"
)

func TestUserMessageWithAttach(t *testing.T) {
	// 无附件：纯字符串
	m := userMessageWithAttach("hello", nil)
	if m.Role != "user" || m.Content != "hello" || len(m.ContentBlocks) != 0 {
		t.Fatalf("unexpected message: %+v", m)
	}

	// 有附件：text 块在前 + file 块
	blocks := []prompt.ContentBlock{prompt.FileBlock("file-api-1")}
	m = userMessageWithAttach("看图", blocks)
	if len(m.ContentBlocks) != 2 {
		t.Fatalf("expected 2 blocks, got %+v", m.ContentBlocks)
	}
	if m.ContentBlocks[0].Type != "text" || m.ContentBlocks[0].Text != "看图" {
		t.Fatalf("first block should be text: %+v", m.ContentBlocks[0])
	}
	if m.ContentBlocks[1].Type != "file" || m.ContentBlocks[1].FileID != "file-api-1" {
		t.Fatalf("second block should be file: %+v", m.ContentBlocks[1])
	}
	// 纯文本字段保留（用于显示/FTS）
	if m.Content != "看图" {
		t.Fatalf("content should keep plain text: %q", m.Content)
	}

	// 有附件但文案为空：只有 file 块
	m = userMessageWithAttach("", blocks)
	if len(m.ContentBlocks) != 1 || m.ContentBlocks[0].Type != "file" {
		t.Fatalf("expected only file block, got %+v", m.ContentBlocks)
	}
}

func TestCleanMessagesForModel(t *testing.T) {
	msgs := []prompt.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "看图", ContentBlocks: []prompt.ContentBlock{
			prompt.TextBlock("看图"), prompt.FileBlock("file-api-2"),
		}},
	}
	// 视觉模型：保留块
	cleanMessagesForModel(msgs, "deepseek-v4-flash-vision-exp")
	if len(msgs[1].ContentBlocks) != 2 {
		t.Fatalf("vision model should keep blocks: %+v", msgs[1])
	}
	// 非视觉模型：剥离块，保留文本
	cleanMessagesForModel(msgs, "deepseek-v4-flash")
	if len(msgs[1].ContentBlocks) != 0 || msgs[1].Content != "看图" {
		t.Fatalf("non-vision model should strip blocks: %+v", msgs[1])
	}
}

func TestUploadAttachmentsNonVision(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("attach", nil, "")
	if err := cmd.Flags().Set("attach", "a.png"); err != nil {
		t.Fatal(err)
	}
	_, err := uploadAttachments(t.Context(), cmd)
	if err == nil || !strings.Contains(err.Error(), "不支持图片输入") {
		t.Fatalf("expected non-vision error, got: %v", err)
	}
}

func TestUploadAttachmentsSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/files" && r.Method != http.MethodPost {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"file-api-777","object":"file","bytes":10,"created_at":1,"filename":"test.png","purpose":"user_data"}`))
	}))
	defer srv.Close()

	// 备份并设置配置
	oldKey, oldURL := config.Get("deepseek-api-key", ""), config.Get("deepseek-base-url", "")
	config.Set("deepseek-api-key", "test-key")
	config.Set("deepseek-base-url", srv.URL)
	defer func() {
		config.Set("deepseek-api-key", oldKey)
		config.Set("deepseek-base-url", oldURL)
	}()

	dir := t.TempDir()
	img := filepath.Join(dir, "test.png")
	if err := os.WriteFile(img, []byte("png-bytes"), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	cmd.Flags().StringSlice("attach", nil, "")
	if err := cmd.Flags().Set("attach", img); err != nil {
		t.Fatal(err)
	}
	ctx := dcontext.WithValue(t.Context(), dcontext.CurrentModelNameKey, "deepseek-v4-flash-vision-exp")
	blocks, err := uploadAttachments(ctx, cmd)
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if len(blocks) != 1 || blocks[0].Type != "file" || blocks[0].FileID != "file-api-777" {
		t.Fatalf("unexpected blocks: %+v", blocks)
	}
}
