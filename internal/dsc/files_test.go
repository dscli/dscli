package dsc

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newFilesTestServer 返回一个 httptest server 并记录收到的请求，便于断言。
func newFilesTestServer(t *testing.T, handler http.HandlerFunc) (*httptest.Server, *FilesAPI) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return srv, NewFilesAPI("test-key", srv.URL)
}

func TestFilesAPIUpload(t *testing.T) {
	var gotMethod, gotPurpose, gotExpiryAnchor, gotExpirySeconds, gotFilename string
	_, api := newFilesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		if r.Method != http.MethodPost || r.URL.Path != "/files" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Errorf("missing auth header")
		}
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			t.Errorf("parse multipart: %v", err)
		}
		gotPurpose = r.FormValue("purpose")
		gotExpiryAnchor = r.FormValue("expires_after[anchor]")
		gotExpirySeconds = r.FormValue("expires_after[seconds]")
		f, fh, err := r.FormFile("file")
		if err != nil {
			t.Errorf("missing file field: %v", err)
		} else {
			gotFilename = fh.Filename
			f.Close()
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id": "file-api-1", "object": "file", "bytes": 1024,
			"created_at": 1700000000, "filename": "example.png", "purpose": "user_data",
			"expires_at": 1700003600,
		})
	})

	// 写一个临时小文件
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "example.png")
	if err := os.WriteFile(imgPath, []byte("fake-png-data"), 0o600); err != nil {
		t.Fatal(err)
	}

	file, err := api.Upload(context.Background(), imgPath, UploadOptions{ExpiresSeconds: 3600})
	if err != nil {
		t.Fatalf("upload failed: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s", gotMethod)
	}
	if gotPurpose != "user_data" {
		t.Errorf("purpose = %q", gotPurpose)
	}
	if gotExpiryAnchor != "created_at" || gotExpirySeconds != "3600" {
		t.Errorf("expiry fields = %q/%q", gotExpiryAnchor, gotExpirySeconds)
	}
	if !strings.Contains(gotFilename, "example.png") {
		t.Errorf("filename not in form: %q", gotFilename)
	}
	if file.ID != "file-api-1" || file.Bytes != 1024 || file.Filename != "example.png" || file.ExpiresAt != 1700003600 {
		t.Errorf("unexpected file object: %+v", file)
	}
}

func TestFilesAPIUploadErrors(t *testing.T) {
	_, api := newFilesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"message": "unsupported file format", "type": "invalid_request_error"},
		})
	})
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "a.png")
	if err := os.WriteFile(imgPath, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := api.Upload(context.Background(), imgPath, UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "unsupported file format") {
		t.Errorf("expected API error message, got: %v", err)
	}

	// 文件不存在
	_, err = api.Upload(context.Background(), filepath.Join(dir, "missing.png"), UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "文件不存在") {
		t.Errorf("expected missing file error, got: %v", err)
	}

	// 超过 64 MiB：用稀疏文件模拟大文件大小
	bigPath := filepath.Join(dir, "big.png")
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	f.Close()
	_, err = api.Upload(context.Background(), bigPath, UploadOptions{})
	if err == nil || !strings.Contains(err.Error(), "64 MiB") {
		t.Errorf("expected size limit error, got: %v", err)
	}
}

func TestFilesAPIList(t *testing.T) {
	var gotQuery string
	_, api := newFilesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]any{
			"object": "list", "data": []map[string]any{
				{"id": "file-api-2", "object": "file", "bytes": 42, "created_at": 1, "filename": "b.png", "purpose": "user_data"},
			},
			"first_id": "file-api-2", "last_id": "file-api-2", "has_more": false,
		})
	})
	list, err := api.List(context.Background(), "file-api-1", 10, "desc", "user_data")
	if err != nil {
		t.Fatal(err)
	}
	want := "after=file-api-1&limit=10&order=desc&purpose=user_data"
	if gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
	if len(list.Data) != 1 || list.Data[0].ID != "file-api-2" || list.HasMore {
		t.Errorf("unexpected list: %+v", list)
	}
}

func TestFilesAPIInfo(t *testing.T) {
	var gotPath, gotMethod string
	_, api := newFilesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		json.NewEncoder(w).Encode(map[string]any{
			"id": "file-api-3", "object": "file", "bytes": 7, "created_at": 2, "filename": "c.gif", "purpose": "user_data",
		})
	})
	file, err := api.Info(context.Background(), "file-api-3")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodGet || gotPath != "/files/file-api-3" {
		t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if file.ID != "file-api-3" || file.Filename != "c.gif" {
		t.Errorf("unexpected file: %+v", file)
	}
}

func TestFilesAPIDelete(t *testing.T) {
	var gotMethod, gotPath string
	_, api := newFilesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewEncoder(w).Encode(map[string]any{"id": "file-api-4", "object": "file", "deleted": true})
	})
	res, err := api.Delete(context.Background(), "file-api-4")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/files/file-api-4" {
		t.Errorf("unexpected request: %s %s", gotMethod, gotPath)
	}
	if !res.Deleted || res.ID != "file-api-4" {
		t.Errorf("unexpected delete result: %+v", res)
	}
}

func TestFilesAPIListBadStatus(t *testing.T) {
	_, api := newFilesTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":{"message":"boom","type":"server_error"}}`))
	})
	_, err := api.List(context.Background(), "", 0, "", "")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected boom error, got: %v", err)
	}
}
