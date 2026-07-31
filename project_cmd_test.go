package main

import (
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
)

func TestProjectListJSON(t *testing.T) {
	origRoot := context.ProjectRoot
	context.ProjectRoot = "/home/user/current-project"
	defer func() { context.ProjectRoot = origRoot }()

	projects := []session.ProjectRow{
		{
			ID:           1,
			ProjectPath:  "/home/user/current-project",
			CreatedAt:    "2026-06-14 07:05:58",
			MaintainerCN: "玻尔",
			MaintainerEN: "Bohr",
			MaintainerID: 30,
		},
		{
			ID:           2,
			ProjectPath:  "/home/user/other-project",
			CreatedAt:    "2026-07-01T08:00:00+08:00",
			MaintainerCN: "",
			MaintainerEN: "",
			MaintainerID: 0,
		},
	}

	// Capture stdout written by projectListJSON.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	jsonErr := projectListJSON(projects)
	w.Close()
	os.Stdout = oldStdout
	if jsonErr != nil {
		t.Fatalf("projectListJSON: %v", jsonErr)
	}
	output, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	r.Close()

	// Verify valid JSON array.
	var parsed []map[string]any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output)
	}
	if len(parsed) != 2 {
		t.Fatalf("expected 2 projects, got %d\n%s", len(parsed), output)
	}

	// First project: full maintainer + is_current marker.
	first := parsed[0]
	if first["id"] != float64(1) {
		t.Errorf("id = %v, want 1", first["id"])
	}
	if first["project_path"] != "/home/user/current-project" {
		t.Errorf("project_path = %v, want raw path without ~ replacement", first["project_path"])
	}
	if first["maintainer_cn"] != "玻尔" {
		t.Errorf("maintainer_cn = %v, want 玻尔", first["maintainer_cn"])
	}
	if first["maintainer_en"] != "Bohr" {
		t.Errorf("maintainer_en = %v, want Bohr", first["maintainer_en"])
	}
	if first["maintainer_id"] != float64(30) {
		t.Errorf("maintainer_id = %v, want 30", first["maintainer_id"])
	}
	if first["is_current"] != true {
		t.Errorf("is_current = %v, want true for current project", first["is_current"])
	}
	// Local time "2006-01-02 15:04:05" must be converted to RFC3339.
	ct, ok := first["created_at"].(string)
	if !ok {
		t.Fatalf("created_at not a string: %v", first["created_at"])
	}
	if _, err := time.Parse(time.RFC3339, ct); err != nil {
		t.Errorf("created_at %q is not RFC3339: %v", ct, err)
	}

	// Second project: no maintainer, not current, RFC3339 passthrough.
	second := parsed[1]
	if second["is_current"] != false {
		t.Errorf("is_current = %v, want false for non-current project", second["is_current"])
	}
	if second["maintainer_cn"] != "" || second["maintainer_id"] != float64(0) {
		t.Errorf("expected empty maintainer fields, got %v / %v", second["maintainer_cn"], second["maintainer_id"])
	}
	if second["created_at"] != "2026-07-01T08:00:00+08:00" {
		t.Errorf("created_at = %v, want RFC3339 passthrough", second["created_at"])
	}
}

func TestProjectListJSONEmpty(t *testing.T) {
	// Empty list must produce "[]", not "null" — easier for client parsing.
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	jsonErr := projectListJSON(nil)
	w.Close()
	os.Stdout = oldStdout
	if jsonErr != nil {
		t.Fatalf("projectListJSON: %v", jsonErr)
	}
	output, _ := io.ReadAll(r)
	r.Close()
	var parsed []any
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output)
	}
	if len(parsed) != 0 {
		t.Errorf("expected empty array, got %d items", len(parsed))
	}
}

func TestFormatProjectTimeRFC3339(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"rfc3339", "2026-07-01T08:00:00+08:00", "2026-07-01T08:00:00+08:00"},
		{"local-format", "2026-06-14 07:05:58", ""}, // 期望可解析为 RFC3339
		{"unparseable", "not-a-time", "not-a-time"}, // 原样返回，不丢数据
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatProjectTimeRFC3339(tt.raw)
			if tt.want != "" {
				if got != tt.want {
					t.Errorf("formatProjectTimeRFC3339(%q) = %q, want %q", tt.raw, got, tt.want)
				}
				return
			}
			if _, err := time.Parse(time.RFC3339, got); err != nil {
				t.Errorf("formatProjectTimeRFC3339(%q) = %q, not RFC3339: %v", tt.raw, got, err)
			}
		})
	}
}
