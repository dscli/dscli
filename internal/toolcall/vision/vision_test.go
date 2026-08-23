package vision

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	dcontext "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/toolcall"
)

// setFilesEndpoint 将 Files API 指向测试服务器并隔离文件缓存
// （避免测试上传写入真实的 ~/.dscli/files.json）。
func setFilesEndpoint(t *testing.T, srv *httptest.Server) {
	t.Helper()
	oldKey := config.Get("deepseek-api-key", "")
	oldURL := config.Get("deepseek-base-url", "")
	oldCache := config.Get("files-cache-path", "")
	config.Set("deepseek-api-key", "test-key")
	config.Set("deepseek-base-url", srv.URL)
	config.Set("files-cache-path", filepath.Join(t.TempDir(), "files.json"))
	t.Cleanup(func() {
		config.Set("deepseek-api-key", oldKey)
		config.Set("deepseek-base-url", oldURL)
		config.Set("files-cache-path", oldCache)
	})
}

func TestToolsRegistered(t *testing.T) {
	names := []string{"vision_file_read", "vision_file_list", "vision_file_info", "vision_file_delete"}
	for _, n := range names {
		if _, ok := toolcall.GetToolDef(t.Context(), n); !ok {
			t.Errorf("tool %q not registered", n)
		}
	}
	// 旧名 alias 兼容：GetToolDef 可解析，但不在工具列表中
	if _, ok := toolcall.GetToolDef(t.Context(), "vision_file_upload"); !ok {
		t.Error("alias vision_file_upload should resolve")
	}
}

func TestHandleReadSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/files" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"file-api-42","object":"file","bytes":8,"created_at":1,"filename":"pic.png","purpose":"user_data"}`))
	}))
	defer srv.Close()
	setFilesEndpoint(t, srv)

	dir := t.TempDir()
	img := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(img, []byte("imagedata"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, _, err := handleRead(t.Context(), toolcall.ToolArgs{"file": img})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	// 非视觉模型（默认 ctx 无模型名）：仅返回普通元数据 JSON
	if !strings.Contains(result, `"id":"file-api-42"`) {
		t.Fatalf("result should contain file id: %s", result)
	}
	if strings.Contains(result, `"dual"`) {
		t.Fatalf("non-vision context should not return dual message: %s", result)
	}
}

func TestHandleReadDualMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"file-api-7","object":"file","bytes":8,"created_at":1,"filename":"pic.png","purpose":"user_data"}`))
	}))
	defer srv.Close()
	setFilesEndpoint(t, srv)

	dir := t.TempDir()
	img := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(img, []byte("imagedata"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 视觉模型：返回 DualMessage（tool 元数据 + user 消息 file 块）
	ctx := context.WithValue(t.Context(), dcontext.CurrentModelNameKey, "deepseek-v4-flash-vision-exp")
	result, _, err := handleRead(ctx, toolcall.ToolArgs{"file": img})
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	toolResult, userMsg, isDual := toolcall.SplitDualResult(result)
	if !isDual {
		t.Fatalf("expected dual message, got: %s", result)
	}
	if !strings.Contains(toolResult, `"id":"file-api-7"`) {
		t.Fatalf("tool result should keep metadata: %s", toolResult)
	}
	if userMsg == nil || userMsg.Role != "user" {
		t.Fatalf("expected user message, got: %+v", userMsg)
	}
	if len(userMsg.ContentBlocks) != 2 {
		t.Fatalf("expected 2 blocks (text+file), got %+v", userMsg.ContentBlocks)
	}
	if userMsg.ContentBlocks[1].Type != "file" || userMsg.ContentBlocks[1].FileID != "file-api-7" {
		t.Fatalf("expected file block, got %+v", userMsg.ContentBlocks[1])
	}
}

func TestHandleReadMissingFile(t *testing.T) {
	if _, _, err := handleRead(t.Context(), toolcall.ToolArgs{}); err == nil {
		t.Fatal("expected error for missing file arg")
	}
}

func TestHandleReadNonexistent(t *testing.T) {
	if _, _, err := handleRead(t.Context(), toolcall.ToolArgs{"file": "/no/such/file.png"}); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestHandleList(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"object":"list","data":[],"first_id":"","last_id":"","has_more":false}`))
	}))
	defer srv.Close()
	setFilesEndpoint(t, srv)

	result, _, err := handleList(t.Context(), toolcall.ToolArgs{"order": "desc", "limit": float64(5)})
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if gotQuery != "limit=5&order=desc" {
		t.Errorf("unexpected query: %s", gotQuery)
	}
	if !strings.Contains(result, `"object":"list"`) {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestHandleInfo(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"file-api-9","object":"file","bytes":1,"created_at":1,"filename":"x.png","purpose":"user_data"}`))
	}))
	defer srv.Close()
	setFilesEndpoint(t, srv)

	result, _, err := handleInfo(t.Context(), toolcall.ToolArgs{"file_id": "file-api-9"})
	if err != nil {
		t.Fatalf("info failed: %v", err)
	}
	if gotPath != "/files/file-api-9" {
		t.Errorf("unexpected path: %s", gotPath)
	}
	if !strings.Contains(result, `"filename":"x.png"`) {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestHandleDelete(t *testing.T) {
	var gotPath, gotMethod string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"file-api-9","object":"file","deleted":true}`))
	}))
	defer srv.Close()
	setFilesEndpoint(t, srv)

	result, _, err := handleDelete(t.Context(), toolcall.ToolArgs{"file_id": "file-api-9"})
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/files/file-api-9" {
		t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if !strings.Contains(result, `"deleted":true`) {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestHandleReadAliasResolves(t *testing.T) {
	// 旧名 vision_file_upload 通过 alias 解析到 vision_file_read 定义
	def, ok := toolcall.GetToolDef(t.Context(), "vision_file_upload")
	if !ok {
		t.Fatal("alias should resolve")
	}
	if def.Name != "vision_file_read" {
		t.Fatalf("alias should map to vision_file_read, got %q", def.Name)
	}
}
