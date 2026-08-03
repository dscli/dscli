// Package aistatus implements the aistatus tool.
//
// aistatus lists all projects with their AI maintainer and status,
// following the same table format as "dscli project list" with an
// added Status column.
//
// Status detection:
//   - on:  lockfile exists and the owning process is alive
//   - nap: process is alive AND the ai_status table has an active nap
//   - off: no running process
package aistatus

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/dscli/dscli/internal/processutil"

	dsctx "github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed aistatus.md
var aistatusMd string

var aistatusTool = toolcall.ToolDef{
	Name:        "aistatus",
	DisplayName: "AI Status",
	Description: aistatusMd,
	Strict:      true,
	Parameters: map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"required":             []string{},
		"additionalProperties": false,
	},
	Category: "ai",
	Handler:  handleAistatus,
}

func init() {
	if err := toolcall.RegisterTool(aistatusTool); err != nil {
		panic(fmt.Sprintf("aistatus: register tool: %v", err))
	}
}

// projectStatus is a row in the output table.
type projectStatus struct {
	ID         string
	Project    string
	Maintainer string
	Status     string
	CreatedAt  string
}

// handleAistatus handles the aistatus tool call.
func handleAistatus(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleAistatus")
	defer span.Finish()

	projects, listErr := session.ListProjects(ctx)
	if listErr != nil {
		return "", "", fmt.Errorf("list projects: %w", listErr)
	}

	if len(projects) == 0 {
		return "没有项目。", "", nil
	}

	// Pre-build nap session map for O(1) lookup.
	napSessions := loadNapSessions(ctx)

	var rows []projectStatus
	home := os.Getenv("HOME")
	currentRoot := dsctx.ProjectRoot

	for _, p := range projects {
		projectPath := p.ProjectPath
		if home != "" {
			projectPath = strings.Replace(projectPath, home, "~", 1)
		}

		maintainer := ""
		if p.MaintainerID > 0 {
			maintainer = fmt.Sprintf("%s(%s, %d)", p.MaintainerCN, p.MaintainerEN, p.MaintainerID)
		}

		idStr := strconv.FormatInt(p.ID, 10)
		if p.ProjectPath == currentRoot {
			idStr = idStr + " →"
		}

		st := determineProjectStatus(p, napSessions)

		created := formatCreatedAt(p.CreatedAt)

		rows = append(rows, projectStatus{
			ID:         idStr,
			Project:    projectPath,
			Maintainer: maintainer,
			Status:     st,
			CreatedAt:  created,
		})
	}

	// Sort: current project first, then by ID.
	sort.SliceStable(rows, func(i, j int) bool {
		if strings.HasSuffix(rows[i].ID, "→") {
			return true
		}
		if strings.HasSuffix(rows[j].ID, "→") {
			return false
		}
		return rows[i].ID < rows[j].ID
	})

	// Format as a tab-aligned table.
	var buf strings.Builder
	w := tabwriter.NewWriter(&buf, 0, 0, 3, ' ', 0)
	fmt.Fprintln(w, "ID\tProject\tMaintainer\tStatus\tCreated At")
	for _, row := range rows {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			row.ID, row.Project, row.Maintainer, row.Status, row.CreatedAt)
	}
	w.Flush()

	result = buf.String()
	return result, warning, nil
}

// loadNapSessions queries the ai_status table and returns a map of
// session_id → nap_until for sessions currently in nap state.
// Errors are silent — the map is simply empty on failure.
func loadNapSessions(ctx context.Context) map[int64]time.Time {
	m := make(map[int64]time.Time)

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return m
	}
	defer db.Close(ctx)

	rows, qErr := db.Query(
		`SELECT session_id, nap_until FROM ai_status WHERE status = 'nap'`,
	)
	if qErr != nil {
		return m
	}
	defer rows.Close()

	for rows.Next() {
		var sid int64
		var napUntilStr string
		if err := rows.Scan(&sid, &napUntilStr); err != nil {
			continue
		}
		if napUntilStr == "" {
			continue
		}
		t, parseErr := time.Parse(time.RFC3339, napUntilStr)
		if parseErr != nil {
			continue
		}
		m[sid] = t
	}
	return m
}

// determineProjectStatus returns "on", "nap (<remaining>)", or "off".
func determineProjectStatus(p session.ProjectRow, napSessions map[int64]time.Time) string {
	lockPath := filepath.Join(p.ProjectPath, ".dscli", "locks", "dscli.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "off"
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid == 0 {
		return "off"
	}

	// Check if process is alive.
	if !processutil.IsAlive(pid) {
		return "off"
	}

	// Process is alive — check nap state.
	if napUntil, ok := napSessions[p.ID]; ok && time.Now().Before(napUntil) {
		remaining := time.Until(napUntil).Round(time.Second)
		return fmt.Sprintf("nap (%s)", remaining)
	}

	return "on"
}

// formatCreatedAt parses and localizes a datetime string.
func formatCreatedAt(raw string) string {
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t, err = time.Parse("2006-01-02 15:04:05", raw)
		if err != nil {
			return raw
		}
	}
	return t.Local().Format(time.DateTime)
}
