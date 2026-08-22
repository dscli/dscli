package vision

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/toolcall"
)

// setFilesEndpoint 将 Files API 指向测试服务器，返回恢复函数。
func setFilesEndpoint(t *testing.T, srv *httptest.Server) {
	t.Helper()
	oldKey := config.Get("deepseek-api-key", "")
	oldURL := config.Get("deepseek-base-url", "")
	config.Set("deepseek-api-key", "test-key")
	config.Set("deepseek-base-url", srv.URL)
	t.Cleanup(func() {
		config.Set("deepseek-api-key", oldKey)
		config.Set("deepseek-base-url", oldURL)
	})
}

func TestToolsRegistered(t *testing.T) {
	names := []string{"vision_file_upload", "vision_file_list", "vision_file_info", "vision_file_delete"}
	for _, n := range names {
		if _, ok := toolcall.GetToolDef(t.Context(), n); !ok {
			t.Errorf("tool %q not registered", n)
		}
	}
}

func TestHandleUploadSuccess(t *testing.T) {
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

	result, _, err := handleUpload(t.Context(), toolcall.ToolArgs{"file": img})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if !strings.Contains(result, `"id":"file-api-42"`) {
		t.Fatalf("result should contain file id: %s", result)
	}
}

func TestHandleUploadMissingFile(t *testing.T) {
	if _, _, err := handleUpload(t.Context(), toolcall.ToolArgs{}); err == nil {
		t.Fatal("expected error for missing file arg")
	}
}

func TestHandleUploadNonexistent(t *testing.T) {
	if _, _, err := handleUpload(t.Context(), toolcall.ToolArgs{"file": "/no/such/file.png"}); err == nil {
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
