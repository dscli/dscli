// Package memory implements persistent memory tools (mem_save, mem_update, mem_search,
// mem_delete, mem_get_observation, mem_stats)
package memory

import (
	"context"
	_ "embed"
	"fmt"

	"gitcode.com/dscli/dscli/internal/memories"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

//go:embed mem_save.md
var mem_save_md string

//go:embed mem_update.md
var mem_update_md string

//go:embed mem_search.md
var mem_search_md string

//go:embed mem_delete.md
var mem_delete_md string

//go:embed mem_get_observation.md
var mem_get_observation_md string

//go:embed mem_stats.md
var mem_stats_md string

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
	// ── Tools ──
	RegisterTool(ToolDef{
		Name:        "mem_save",
		Description: mem_save_md,
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "Short searchable title, e.g. 'JWT auth middleware'"},
				"content": map[string]any{"type": "string", "description": "Detailed content, recommended format: **What**/**Why**/**Where**/**Learned**"},
				"type":    map[string]any{"type": "string", "description": "Type: decision, architecture, bugfix, pattern, config, discovery, learning"},
			},
			"required":             []string{"title", "content"},
			"additionalProperties": false,
		},
		Handler: handleMemSave,
	})

	RegisterTool(ToolDef{
		Name:        "mem_update",
		Description: mem_update_md,
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "integer", "description": "Memory ID to update (required)"},
				"title":   map[string]any{"type": "string", "description": "New title (optional)"},
				"content": map[string]any{"type": "string", "description": "New content (optional)"},
				"type":    map[string]any{"type": "string", "description": "New type (optional)"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Handler: handleMemUpdate,
	})

	RegisterTool(ToolDef{
		Name:        "mem_search",
		Description: mem_search_md,
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "Search query — natural language or keywords (required)"},
				"type":  map[string]any{"type": "string", "description": "Filter by type: decision, architecture, bugfix, pattern, config, discovery, learning"},
				"limit": map[string]any{"type": "integer", "description": "Max results (default 10, max 50)"},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Handler: handleMemSearch,
	})

	RegisterTool(ToolDef{
		Name:        "mem_delete",
		Description: mem_delete_md,
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "Memory ID to delete (required)"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Handler: handleMemDelete,
	})

	RegisterTool(ToolDef{
		Name:        "mem_get_observation",
		Description: mem_get_observation_md,
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "Memory ID (required)"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Handler: handleMemGetObservation,
	})

	RegisterTool(ToolDef{
		Name:        "mem_stats",
		Description: mem_stats_md,
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type":                 "object",
			"properties":           map[string]any{},
			"additionalProperties": false,
		},
		Handler: handleMemStats,
	})
}

// ─── Handlers ─────────────────────────────────────────────────────────────────

// handleMemSave saves a new memory observation.
func handleMemSave(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	title := ToolArgsValue(args, "title", "")
	body := ToolArgsValue(args, "content", "")
	typ := ToolArgsValue(args, "type", "manual")

	if title == "" || body == "" {
		err = fmt.Errorf("title and content are required")
		return result, warning, err
	}

	result, warning, err = memories.HandleMemSave(ctx, title, body, typ)
	return result, warning, err
}

// handleMemUpdate updates an existing memory by ID.
func handleMemUpdate(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id is required")
		return result, warning, err
	}

	// Build update with provided fields only
	title := ToolArgsValue(args, "title", "")
	body := ToolArgsValue(args, "content", "")
	typ := ToolArgsValue(args, "type", "")
	result, warning, err = memories.HandleMemUpdate(ctx, id, title, body, typ)
	return result, warning, err
}

// handleMemSearch searches memories using FTS5 full-text search.
func handleMemSearch(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	query := ToolArgsValue(args, "query", "")
	typ := ToolArgsValue(args, "type", "")
	limit := ToolArgsValue(args, "limit", 10)
	limit = min(limit, 50)
	if limit <= 0 {
		limit = 10
	}

	if query == "" {
		err = fmt.Errorf("query is required")
		return result, warning, err
	}

	result, warning, err = memories.HandleMemSearch(ctx, query, typ, limit)
	return result, warning, err
}

// handleMemDelete deletes a memory by ID.
func handleMemDelete(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id is required")
		return result, warning, err
	}
	result, warning, err = memories.HandleMemDelete(ctx, id)
	return result, warning, err
}

// handleMemGetObservation retrieves full memory content by ID.
// Unlike mem_search which returns truncated previews, this returns the complete content.
func handleMemGetObservation(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id is required")
		return result, warning, err
	}
	result, warning, err = memories.HandleMemGetObservation(ctx, id)
	return result, warning, err
}

// handleMemStats returns memory system statistics.
func handleMemStats(ctx context.Context, _ ToolArgs) (result, warning string, err error) {
	result, warning, err = memories.HandleMemStats(ctx)
	return result, warning, err
}
