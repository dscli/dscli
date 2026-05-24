// Package mail registers mail tools (sendmail/readmail/mail_search/maintainers)
// with the toolcall framework and parses LLM-issued tool arguments.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Layering
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// This package is a thin adapter between LLM tool calls and the core mail
// logic in internal/mail. It owns:
//
//   - ToolDef registration (name, description, JSON Schema parameters)
//   - Argument extraction from map[string]any with defaults and validation
//   - Delegation to mail.Handle* functions
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Tools
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
//	─ sendmail     — Send mail to another maintainer by name/email
//	─ readmail     — Read mails (list or single) for current maintainer
//	─ mail_search  — FTS5 search across mails
//	─ maintainers  — List all known maintainers
package mail

import (
	"context"
	_ "embed"
	"fmt"

	mailcore "gitcode.com/dscli/dscli/internal/mail"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed sendmail.md
var sendmail_md string

//go:embed readmail.md
var readmail_md string

//go:embed mail_search.md
var mail_search_md string

//go:embed maintainers.md
var maintainers_md string

var RegisterTool = toolcall.RegisterTool

type (
	ToolArgs  = toolcall.ToolArgs
	ToolDef   = toolcall.ToolDef
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}

func init() {
	RegisterTool(ToolDef{
		Name:        "sendmail",
		Description: sendmail_md,
		Category:    "mail",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"recipient": map[string]any{"type": "string", "description": "Recipient name or email (required)"},
				"subject":   map[string]any{"type": "string", "description": "Mail subject (optional)"},
				"body":      map[string]any{"type": "string", "description": "Mail body (optional, but subject or body required)"},
			},
			"required":             []string{"recipient"},
			"additionalProperties": false,
		},
		Handler: handleSendMail,
	})

	RegisterTool(ToolDef{
		Name:        "readmail",
		Description: readmail_md,
		Category:    "mail",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":          map[string]any{"type": "integer", "description": "Mail ID to read (optional, omit to list)"},
				"unread_only": map[string]any{"type": "boolean", "description": "Show only unread mails (default false)"},
				"limit":       map[string]any{"type": "integer", "description": "Max results for list view (default 20, max 100)"},
			},
			"additionalProperties": false,
		},
		Handler: handleReadMail,
	})

	RegisterTool(ToolDef{
		Name:        "mail_search",
		Description: mail_search_md,
		Category:    "mail",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query (required)"},
				"limit": map[string]any{"type": "integer", "description": "Max results (default 10, max 50)"},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Handler: handleMailSearch,
	})

	RegisterTool(ToolDef{
		Name:        "maintainers",
		Description: maintainers_md,
		Category:    "mail",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Handler: handleMaintainers,
	})
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

func handleSendMail(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	recipient := ToolArgsValue(args, "recipient", "")
	subject := ToolArgsValue(args, "subject", "")
	body := ToolArgsValue(args, "body", "")

	if recipient == "" {
		err = fmt.Errorf("recipient is required")
		return
	}
	if subject == "" && body == "" {
		err = fmt.Errorf("subject or body is required")
		return
	}

	result, warning, err = mailcore.HandleSendMail(ctx, recipient, subject, body)
	return
}

func handleReadMail(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	mid := ToolArgsValue(args, "id", int64(0))
	unreadOnly := ToolArgsValue(args, "unread_only", false)
	limit := ToolArgsValue(args, "limit", 20)

	result, warning, err = mailcore.HandleReadMail(ctx, mid, unreadOnly, limit)
	return
}

func handleMailSearch(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	query := ToolArgsValue(args, "query", "")
	limit := ToolArgsValue(args, "limit", 10)
	limit = min(limit, 50)
	if limit <= 0 {
		limit = 10
	}

	if query == "" {
		err = fmt.Errorf("query is required")
		return
	}

	result, warning, err = mailcore.HandleMailSearch(ctx, query, limit)
	return
}

func handleMaintainers(ctx context.Context, _ ToolArgs) (result, warning string, err error) {
	result, warning, err = mailcore.HandleMaintainers(ctx)
	return
}