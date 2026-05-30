package sql

import (
	"strings"
	"testing"
	"time"
)

func TestTranslateQuery(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantPrefix string // expected SQL prefix
		wantErr    bool
	}{
		{
			name:       ".tables",
			query:      ".tables",
			wantPrefix: "SELECT name FROM sqlite_master WHERE type='table'",
		},
		{
			name:       ".schema (no arg)",
			query:      ".schema",
			wantPrefix: "SELECT sql FROM sqlite_master WHERE type='table'",
		},
		{
			name:       ".schema with table name",
			query:      ".schema memories",
			wantPrefix: "SELECT sql FROM sqlite_master WHERE type='table' AND name='memories'",
		},
		{
			name:       ".indices",
			query:      ".indices memories",
			wantPrefix: "SELECT name FROM sqlite_master WHERE type='index' AND tbl_name='memories'",
		},
		{
			name:    ".indices without arg",
			query:   ".indices",
			wantErr: true,
		},
		{
			name:       "plain SELECT passes through",
			query:      "SELECT 1",
			wantPrefix: "SELECT 1",
		},
		{
			name:    "unknown dot-command",
			query:   ".foobar",
			wantErr: true,
		},
		{
			name:       "trailing semicolon stripped in check (passes through)",
			query:      "SELECT 1;",
			wantPrefix: "SELECT 1;",
		},
		{
			name:    ".tables with extraneous arg",
			query:   ".tables foo",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := translateQuery(tt.query)
			if tt.wantErr {
				if err == nil {
					t.Errorf("translateQuery(%q) expected error, got nil", tt.query)
				}
				return
			}
			if err != nil {
				t.Errorf("translateQuery(%q) unexpected error: %v", tt.query, err)
				return
			}
			if !strings.HasPrefix(got, tt.wantPrefix) {
				t.Errorf("translateQuery(%q) = %q, want prefix %q", tt.query, got, tt.wantPrefix)
			}
		})
	}
}

func TestCheckReadOnly(t *testing.T) {
	tests := []struct {
		name    string
		sql     string
		wantErr bool
	}{
		{"SELECT", "SELECT * FROM foo", false},
		{"select (lowercase)", "select * from foo", false},
		{"SELECT with JOIN", "SELECT a.x, b.y FROM a JOIN b ON a.id = b.id", false},
		{"EXPLAIN", "EXPLAIN SELECT 1", false},
		{"EXPLAIN QUERY PLAN", "EXPLAIN QUERY PLAN SELECT 1", false},
		{"PRAGMA table_info", "PRAGMA table_info('memories')", false},
		{"PRAGMA index_list", "PRAGMA index_list", false},
		{"INSERT rejected", "INSERT INTO foo VALUES(1)", true},
		{"UPDATE rejected", "UPDATE foo SET x=1", true},
		{"DELETE rejected", "DELETE FROM foo", true},
		{"DROP rejected", "DROP TABLE foo", true},
		{"ALTER rejected", "ALTER TABLE foo ADD COLUMN x", true},
		{"CREATE rejected", "CREATE TABLE foo(x)", true},
		{"BEGIN rejected", "BEGIN TRANSACTION", true},
		{"COMMIT rejected", "COMMIT", true},
		{"multi-statement rejected", "SELECT 1; DROP TABLE foo", true},
		{"trailing semicolon allowed", "SELECT 1;", false},
		{"ATTACH rejected", "ATTACH 'other.db' AS other", true},
		{"VACUUM rejected", "VACUUM", true},
		{"read-only PRAGMA stats", "PRAGMA stats", false},
		{"write PRAGMA cache_size (equals)", "PRAGMA cache_size = -2000", true},
		{"write PRAGMA cache_size (parens)", "PRAGMA cache_size(2000)", true},
		{"write PRAGMA busy_timeout (parens)", "PRAGMA busy_timeout(5000)", true},
		{"read-only PRAGMA integrity_check with arg", "PRAGMA integrity_check('main')", false},
		{"empty query", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkReadOnly(tt.sql)
			if tt.wantErr && err == nil {
				t.Errorf("checkReadOnly(%q) expected error, got nil", tt.sql)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("checkReadOnly(%q) unexpected error: %v", tt.sql, err)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
		want  string
	}{
		{"nil", nil, "NULL"},
		{"string", "hello", "hello"},
		{"[]byte", []byte("hello"), "hello"},
		{"int64", int64(42), "42"},
		{"float64", float64(3.14), "3.14"},
		{"bool true", true, "true"},
		{"bool false", false, "false"},
		{"time", time.Date(2026, 5, 27, 12, 0, 0, 0, time.UTC), "2026-05-27 12:00:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatValue(tt.value)
			if got != tt.want {
				t.Errorf("formatValue(%v) = %q, want %q", tt.value, got, tt.want)
			}
		})
	}
}

func TestQuoteString(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "memories", "'memories'"},
		{"with single quote", "foo'bar", "'foo''bar'"},
		{"empty", "", "''"},
		{"with double quote", `foo"bar`, `'foo"bar'`},
		{"with semicolon", "foo;bar", "'foo;bar'"},
		{"with backslash", `foo\bar`, `'foo\bar'`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := quoteString(tt.input)
			if got != tt.want {
				t.Errorf("quoteString(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPadRight(t *testing.T) {
	tests := []struct {
		name  string
		s     string
		width int
		want  string
	}{
		{"shorter", "foo", 6, "foo   "},
		{"equal", "foo", 3, "foo"},
		{"longer", "foobar", 3, "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := padRight(tt.s, tt.width)
			if got != tt.want {
				t.Errorf("padRight(%q, %d) = %q, want %q", tt.s, tt.width, got, tt.want)
			}
		})
	}
}

func TestMergeWarning(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		want string
	}{
		{"both empty", "", "", ""},
		{"a only", "warn a", "", "warn a"},
		{"b only", "", "warn b", "warn b"},
		{"both", "warn a", "warn b", "warn a\nwarn b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mergeWarning(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("mergeWarning(%q, %q) = %q, want %q", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
