// Package ainap implements the ainap tool.
//
// ainap puts the current AI session to sleep for a specified duration.
// It records the nap state in the ai_status table so that aistatus
// can report the session as "nap" to other AIs.
//
// The implementation is deliberately simple: it writes the nap state
// to the database, blocks with time.Sleep, then restores the state
// to "on" on wake.
package ainap

import (
	_ "embed"
	"context"
	"fmt"
	"time"

	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/dscli/dscli/internal/toolcall"
	"github.com/nanjj/clog"
)

//go:embed ainap.md
var ainapMd string

var ainapTool = toolcall.ToolDef{
	Name:        "ainap",
	DisplayName: "AI Nap",
	Description: ainapMd,
	Strict:      true,
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"seconds": map[string]any{
				"type":        "integer",
				"description": "Number of seconds to sleep (required)",
			},
			"reason": map[string]any{
				"type":        "string",
				"description": "Reason for sleeping (optional)",
			},
		},
		"required":             []string{"seconds"},
		"additionalProperties": false,
	},
	Category: "utility",
	Handler:  handleAinap,
}

func init() {
	if err := toolcall.RegisterTool(ainapTool); err != nil {
		panic(fmt.Sprintf("ainap: register tool: %v", err))
	}
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS ai_status (
			session_id INTEGER PRIMARY KEY,
			status TEXT NOT NULL DEFAULT 'off',
			nap_until DATETIME,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id) ON DELETE CASCADE
		)`,
	)
}

// handleAinap handles the ainap tool call.
func handleAinap(ctx context.Context, args toolcall.ToolArgs) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "handleAinap")
	defer span.Finish()

	seconds := toolcall.ToolArgsValue(args, "seconds", int64(30))
	reason := toolcall.ToolArgsValue(args, "reason", "")

	if seconds <= 0 {
		seconds = 1
	}

	sessionID := session.GetCurrentSessionID(ctx)
	napUntil := time.Now().Add(time.Duration(seconds) * time.Second)

	// Record nap state so aistatus can show it.
	db, dbErr := sqlite.OpenDB(ctx)
	if dbErr == nil {
		_, _ = db.Exec(`
			INSERT INTO ai_status (session_id, status, nap_until, updated_at)
			VALUES (?, 'nap', ?, CURRENT_TIMESTAMP)
			ON CONFLICT(session_id) DO UPDATE SET
				status        = 'nap',
				nap_until     = ?,
				updated_at    = CURRENT_TIMESTAMP`,
			sessionID, napUntil.Format(time.RFC3339), napUntil.Format(time.RFC3339))
		db.Close(ctx)
	}

	// Blocking sleep — the simple approach.
	time.Sleep(time.Duration(seconds) * time.Second)

	// Restore state after waking.
	db, dbErr = sqlite.OpenDB(ctx)
	if dbErr == nil {
		_, _ = db.Exec(`
			INSERT INTO ai_status (session_id, status, updated_at)
			VALUES (?, 'on', CURRENT_TIMESTAMP)
			ON CONFLICT(session_id) DO UPDATE SET
				status     = 'on',
				nap_until  = NULL,
				updated_at = CURRENT_TIMESTAMP`,
			sessionID)
		db.Close(ctx)
	}

	if reason != "" {
		result = fmt.Sprintf("醒了（睡了 %d 秒），原因：%s", seconds, reason)
	} else {
		result = fmt.Sprintf("醒了（睡了 %d 秒）", seconds)
	}

	return result, warning, nil
}
