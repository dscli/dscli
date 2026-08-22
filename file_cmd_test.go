package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dscli/dscli/internal/config"
	"github.com/spf13/cobra"
)

// setupFileTestConfig 注入测试专用的 API 参数并隔离缓存路径；
// 测试结束自动清理，避免影响开发机真实配置。
func setupFileTestConfig(t *testing.T, srvURL string) {
	t.Helper()
	config.SetValue("deepseek-api-key", "test-key")
	config.SetValue("deepseek-base-url", srvURL)
	config.SetValue("files-cache-path", filepath.Join(t.TempDir(), "files.json"))
	t.Cleanup(func() {
		config.SetValue("deepseek-api-key", nil)
		config.SetValue("deepseek-base-url", nil)
		config.SetValue("files-cache-path", nil)
	})
}

// runFileSub 通过构造好的子命令执行并捕获 stdout。
func runFileSub(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

// fileHandler 返回一个记录请求的 Files API mock server。
func fileHandler(t *testing.T, uploads *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/files":
			if uploads != nil {
				*uploads++
			}
			json.NewEncoder(w).Encode(map[string]any{
				"id": "file-cli-1", "object": "file", "bytes": 4,
				"created_at": 1700000000, "filename": "x.png", "purpose": "user_data",
			})
		case r.Method == http.MethodGet && r.URL.Path == "/files":
			json.NewEncoder(w).Encode(map[string]any{
				"object": "list", "data": []map[string]any{
					{"id": "file-cli-1", "object": "file", "bytes": 4, "created_at": 1700000000, "filename": "x.png", "purpose": "user_data"},
					{"id": "file-cli-2", "object": "file", "bytes": 9, "created_at": 1700000001, "filename": "y.png", "purpose": "user_data"},
				},
				"first_id": "file-cli-1", "last_id": "file-cli-2", "has_more": false,
			})
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/files/"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": strings.TrimPrefix(r.URL.Path, "/files/"), "object": "file",
				"bytes": 4, "created_at": 1700000000, "filename": "x.png", "purpose": "user_data",
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/files/"):
			json.NewEncoder(w).Encode(map[string]any{
				"id": strings.TrimPrefix(r.URL.Path, "/files/"), "object": "file", "deleted": true,
			})
		default:
			http.Error(w, `{"error":{"message":"unexpected request"}}`, http.StatusNotFound)
		}
	}))
}

func TestFileUploadRunEPrintsID(t *testing.T) {
	srv := fileHandler(t, nil)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	imgPath := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(imgPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runFileSub(t, newFileUploadCmd(), imgPath)
	if err != nil {
		t.Fatalf("upload error: %v", err)
	}
	if strings.TrimSpace(out) != "file-cli-1" {
		t.Errorf("stdout = %q, want file-cli-1", out)
	}
}

func TestFileUploadRunEJSON(t *testing.T) {
	srv := fileHandler(t, nil)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	imgPath := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(imgPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := runFileSub(t, newFileUploadCmd(), "--format", "json", imgPath)
	if err != nil {
		t.Fatalf("upload error: %v", err)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(out), &obj); err != nil {
		t.Fatalf("json output invalid: %q (%v)", out, err)
	}
	if obj["id"] != "file-cli-1" {
		t.Errorf("json id = %v, want file-cli-1", obj["id"])
	}
}

func TestFileUploadUsesCache(t *testing.T) {
	uploads := 0
	srv := fileHandler(t, &uploads)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	imgPath := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(imgPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	first, err := runFileSub(t, newFileUploadCmd(), imgPath)
	if err != nil {
		t.Fatalf("first upload: %v", err)
	}
	second, err := runFileSub(t, newFileUploadCmd(), imgPath)
	if err != nil {
		t.Fatalf("second upload: %v", err)
	}
	if uploads != 1 {
		t.Errorf("uploads = %d, want 1 (second must hit cache)", uploads)
	}
	if first != second {
		t.Errorf("second upload = %q, want same id %q", second, first)
	}
}

func TestFileListRunE(t *testing.T) {
	srv := fileHandler(t, nil)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	out, err := runFileSub(t, newFileListCmd())
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	if !strings.Contains(out, "file-cli-1") || !strings.Contains(out, "file-cli-2") {
		t.Errorf("table output missing file ids: %q", out)
	}
	if !strings.Contains(out, "x.png") {
		t.Errorf("table output missing filename: %q", out)
	}
}

func TestFileListRunEJSON(t *testing.T) {
	srv := fileHandler(t, nil)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	out, err := runFileSub(t, newFileListCmd(), "--format", "json")
	if err != nil {
		t.Fatalf("list error: %v", err)
	}
	var arr []map[string]any
	if err := json.Unmarshal([]byte(out), &arr); err != nil {
		t.Fatalf("json output invalid: %q (%v)", out, err)
	}
	if len(arr) != 2 || arr[0]["id"] != "file-cli-1" {
		t.Errorf("unexpected json list: %v", arr)
	}
}

func TestFileInfoRunE(t *testing.T) {
	srv := fileHandler(t, nil)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	out, err := runFileSub(t, newFileInfoCmd(), "file-cli-1")
	if err != nil {
		t.Fatalf("info error: %v", err)
	}
	if !strings.Contains(out, "file-cli-1") {
		t.Errorf("info output missing id: %q", out)
	}
}

func TestFileDeleteRunE(t *testing.T) {
	srv := fileHandler(t, nil)
	defer srv.Close()
	setupFileTestConfig(t, srv.URL)

	out, err := runFileSub(t, newFileDeleteCmd(), "file-cli-1")
	if err != nil {
		t.Fatalf("delete error: %v", err)
	}
	if strings.TrimSpace(out) != "deleted file-cli-1" {
		t.Errorf("stdout = %q, want deleted file-cli-1", out)
	}
}

func TestFileTimeFormat(t *testing.T) {
	// 0（未知/永久）显示为空。
	if got := formatFileTime(0); got != "" {
		t.Errorf("formatFileTime(0) = %q, want empty", got)
	}
	// 非零按本地时区渲染为 "2006-01-02 15:04"；只校验格式长度，
	// 不硬编码具体时间（开发机时区不定）。
	got := formatFileTime(1700000000)
	if len(got) != len("2006-01-02 15:04") || !strings.Contains(got, "-") {
		t.Errorf("formatFileTime(1700000000) = %q, want datetime format", got)
	}
}
