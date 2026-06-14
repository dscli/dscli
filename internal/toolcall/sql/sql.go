package sql

import (
	"database/sql"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/dscli/dscli/internal/toolcall"
)

//go:embed sql_sql.md
var toolDescription string

// dotCommand maps sqlite3 CLI dot-commands to SQL queries.
type dotCommand struct {
	sql   string
	noArg bool
}

var dotCommands = map[string]dotCommand{
	".schema": {"SELECT sql FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name", true},
	".tables": {"SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name", true},
}

// readOnlyPragmas lists PRAGMA commands that are safe for read-only access.
// Some PRAGMAs become write operations when given arguments (e.g. cache_size).
// Those are listed in readOnlyPragmasNoArgs — allowed without args, rejected with args.
var readOnlyPragmas = map[string]bool{
	// Always read-only (even with arguments in parentheses)
	"table_info":        true,
	"index_list":        true,
	"index_info":        true,
	"foreign_key_list":  true,
	"table_xinfo":       true,
	"index_xinfo":       true,
	"quick_check":       true,
	"integrity_check":   true,
	"foreign_key_check": true,

	// Read-only without arguments (rejected if args present)
	"compile_options":           true,
	"database_list":             true,
	"collation_list":            true,
	"function_list":             true,
	"module_list":               true,
	"pragma_list":               true,
	"stats":                     true,
	"page_count":                true,
	"page_size":                 true,
	"freelist_count":            true,
	"schema_version":            true,
	"user_version":              true,
	"application_id":            true,
	"auto_vacuum":               true,
	"busy_timeout":              true,
	"cache_size":                true,
	"encoding":                  true,
	"journal_mode":              true,
	"legacy_file_format":        true,
	"locking_mode":              true,
	"max_page_count":            true,
	"read_uncommitted":          true,
	"recursive_triggers":        true,
	"reverse_unordered_selects": true,
	"secure_delete":             true,
	"soft_heap_limit":           true,
	"synchronous":               true,
	"temp_store":                true,
	"threads":                   true,
	"wal_autocheckpoint":        true,
	"data_version":              true,
}

// pragmasWithArgs lists PRAGMAs that remain read-only even with arguments.
var pragmasWithArgs = map[string]bool{
	"table_info":        true,
	"index_list":        true,
	"index_info":        true,
	"foreign_key_list":  true,
	"table_xinfo":       true,
	"index_xinfo":       true,
	"quick_check":       true,
	"integrity_check":   true,
	"foreign_key_check": true,
}

func init() {
	toolcall.RegisterTool(toolcall.ToolDef{
		Name:        "sql",
		Description: toolDescription,
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "SQL query or dot-command (e.g. .tables, .schema, SELECT ...)",
				},
				"timeout": map[string]any{
					"type":        "integer",
					"description": "Timeout in seconds (default 30)",
				},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},

		Category: "system",
		Timeout:  30 * time.Second,
		Handler:  handleSQL,
	})
}

// handleSQL executes a read-only SQL query or dot-command.
func handleSQL(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	query := toolcall.ToolArgsValue(args, "query", "")
	if query == "" {
		return "", "", fmt.Errorf("query is required")
	}

	// Translate dot-commands to SQL.
	translated, userWarning, transErr := translateQuery(query)
	if transErr != nil {
		return "", "", transErr
	}
	if userWarning != "" {
		warning = userWarning
	}

	// Safety check: only allow SELECT / EXPLAIN / read-only PRAGMA.
	if err := checkReadOnly(translated); err != nil {
		// Return as warning so LLM can retry with a corrected query.
		warning = mergeWarning(warning, err.Error())
		return "", warning, nil
	}

	// Execute query against the existing DB singleton.
	db, err := sqlite.OpenDB()
	if err != nil {
		return "", "", fmt.Errorf("open database: %w", err)
	}
	defer db.Close()
	rows, err := db.Query(translated)
	if err != nil {
		// SQL syntax errors → warning (LLM can correct and retry).
		warning = mergeWarning(warning, fmt.Sprintf("SQL error: %v", err))
		return "", warning, nil
	}
	defer rows.Close()

	// Format result as psql-style table.
	table, rowCount, err := formatTable(rows)
	if err != nil {
		return "", "", fmt.Errorf("format result: %w", err)
	}

	var sb strings.Builder
	sb.WriteString(table)
	sb.WriteString(fmt.Sprintf("(%d rows)", rowCount))

	result = sb.String()
	return result, warning, nil
}

// translateQuery translates sqlite3 dot-commands to SQL.
// translateQuery translates sqlite3 dot-commands to SQL.
func translateQuery(query string) (string, string, error) {
	q := strings.TrimSpace(query)

	if strings.HasPrefix(q, ".") {
		parts := strings.SplitN(q, " ", 2)
		cmd := strings.ToLower(parts[0])
		arg := ""
		if len(parts) > 1 {
			arg = strings.TrimSpace(parts[1])
		}

		// .schema <name> — special case with argument.
		if cmd == ".schema" && arg != "" {
			return fmt.Sprintf(
				"SELECT sql FROM sqlite_master WHERE type='table' AND name=%s",
				quoteString(arg)), "", nil
		}

		// .indices <table>
		if cmd == ".indices" {
			if arg == "" {
				return "", "", fmt.Errorf(".indices requires a table name")
			}
			return fmt.Sprintf(
				"SELECT name FROM sqlite_master WHERE type='index' AND tbl_name=%s ORDER BY name",
				quoteString(arg)), "", nil
		}

		dc, ok := dotCommands[cmd]
		if !ok {
			return "", "", fmt.Errorf("unknown dot-command: %s (supported: .schema, .tables, .indices <table>)", cmd)
		}

		if dc.noArg && arg != "" {
			return "", "", fmt.Errorf("dot-command %s does not accept arguments", cmd)
		}

		return dc.sql, "", nil
	}

	return q, "", nil
}

// checkReadOnly validates that the SQL is read-only.
// checkReadOnly validates that the SQL is read-only.
func checkReadOnly(sqlStr string) error {
	normalized := strings.TrimSpace(sqlStr)
	if normalized == "" {
		return fmt.Errorf("empty query")
	}

	// Strip trailing semicolons first, then check for remaining
	// semicolons (multi-statement injection). Note: this may
	// falsely reject semicolons inside string literals (e.g.
	// SELECT 'hello;world'), but such queries are rare and the
	// LLM can retry with a corrected query.
	for strings.HasSuffix(normalized, ";") {
		normalized = strings.TrimSpace(strings.TrimSuffix(normalized, ";"))
	}
	if strings.Contains(normalized, ";") {
		return fmt.Errorf("multi-statement queries are not allowed")
	}

	upper := strings.ToUpper(normalized)

	// Allow SELECT.
	if strings.HasPrefix(upper, "SELECT") {
		return nil
	}

	// Allow EXPLAIN / EXPLAIN QUERY PLAN.
	if strings.HasPrefix(upper, "EXPLAIN") {
		return nil
	}

	// Allow read-only PRAGMA.
	if strings.HasPrefix(upper, "PRAGMA") {
		rest := strings.TrimSpace(normalized[6:])
		// Strip optional schema prefix: PRAGMA schema.table_info → table_info
		if idx := strings.IndexByte(rest, '.'); idx >= 0 {
			rest = rest[idx+1:]
		}

		// Reject any PRAGMA with assignment (=) — always a write.
		if strings.Contains(rest, "=") {
			return fmt.Errorf("PRAGMA with assignment is not allowed")
		}

		// Extract pragma name (strip arguments if present).
		hasArgs := strings.Contains(rest, "(")
		pragmaName := rest
		if idx := strings.IndexByte(pragmaName, '('); idx >= 0 {
			pragmaName = pragmaName[:idx]
		}
		pragmaName = strings.TrimSpace(strings.ToLower(pragmaName))

		if !readOnlyPragmas[pragmaName] {
			return fmt.Errorf("PRAGMA %q is not allowed (read-only PRAGMAs only)", pragmaName)
		}

		// PRAGMAs that become writes when given arguments (e.g. cache_size(2000))
		// are rejected unless explicitly listed as safe with args.
		if hasArgs && !pragmasWithArgs[pragmaName] {
			return fmt.Errorf("PRAGMA %q with arguments is not allowed (would be a write)", pragmaName)
		}

		return nil
	}

	// Reject dangerous statements.
	dangerous := []string{
		"INSERT", "UPDATE", "DELETE", "DROP", "ALTER", "CREATE",
		"ATTACH", "DETACH", "REINDEX", "VACUUM", "ANALYZE",
		"BEGIN", "COMMIT", "ROLLBACK", "SAVEPOINT", "RELEASE",
		"REPLACE", "UPSERT",
	}
	for _, kw := range dangerous {
		if strings.HasPrefix(upper, kw) {
			return fmt.Errorf("only SELECT queries are allowed, got: %s", kw)
		}
	}

	return fmt.Errorf("unrecognized query type: only SELECT, EXPLAIN, and read-only PRAGMA are supported")
}

// formatTable renders query rows as a psql-style aligned table.
// formatTable renders query rows as a psql-style aligned table.
func formatTable(rows *sql.Rows) (string, int, error) {
	columns, err := rows.Columns()
	if err != nil {
		return "", 0, fmt.Errorf("get columns: %w", err)
	}

	const (
		maxRows     = 50
		maxColWidth = 500
		maxTotal    = 10000
	)

	// Build column width trackers from header names.
	colWidths := make([]int, len(columns))
	for i, c := range columns {
		colWidths[i] = len(c)
	}

	var data [][]string
	rowCount := 0
	totalChars := 0
	truncated := false
	truncatedRows := 0

	for rows.Next() {
		if rowCount >= maxRows {
			truncated = true
			// Count remaining rows by advancing cursor only.
			for rows.Next() {
				truncatedRows++
			}
			break
		}

		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return "", 0, fmt.Errorf("scan row: %w", err)
		}

		row := make([]string, len(columns))
		rowChars := 0
		for i, v := range values {
			s := formatValue(v)
			if len(s) > maxColWidth {
				s = s[:maxColWidth] + "…"
			}
			row[i] = s
			if len(s) > colWidths[i] {
				colWidths[i] = len(s)
			}
			rowChars += len(s)
		}
		data = append(data, row)
		rowCount++
		totalChars += rowChars

		if totalChars > maxTotal {
			// Count remaining rows.
			for rows.Next() {
				truncatedRows++
			}
			truncated = true
			break
		}
	}

	if err := rows.Err(); err != nil {
		return "", 0, fmt.Errorf("rows iteration error: %w", err)
	}

	var sb strings.Builder

	// Header row.
	for i, col := range columns {
		if i > 0 {
			sb.WriteString(" | ")
		}
		sb.WriteString(padRight(col, colWidths[i]))
	}
	sb.WriteByte('\n')

	// Separator.
	for i, w := range colWidths {
		if i > 0 {
			sb.WriteString("-+-")
		}
		sb.WriteString(strings.Repeat("-", w))
	}
	sb.WriteByte('\n')

	// Data rows.
	for _, row := range data {
		for i, v := range row {
			if i > 0 {
				sb.WriteString(" | ")
			}
			sb.WriteString(padRight(v, colWidths[i]))
		}
		sb.WriteByte('\n')
	}

	if truncated {
		if truncatedRows > 0 {
			sb.WriteString(fmt.Sprintf("… (truncated: %d more rows", truncatedRows))
		} else {
			sb.WriteString("… (truncated")
		}
		sb.WriteString(")\n")
	}

	return sb.String(), rowCount, nil
}

// formatValue converts a database value to its string representation.
func formatValue(v interface{}) string {
	if v == nil {
		return "NULL"
	}
	switch t := v.(type) {
	case []byte:
		return string(t)
	case string:
		return t
	case int64:
		return fmt.Sprintf("%d", t)
	case float64:
		return fmt.Sprintf("%g", t)
	case bool:
		return fmt.Sprintf("%t", t)
	case time.Time:
		return t.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprintf("%v", t)
	}
}

func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// quoteString returns an SQLite string literal for safe use in queries.
// Single quotes are escaped by doubling (SQLite standard).
func quoteString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

func mergeWarning(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return a + "\n" + b
}
