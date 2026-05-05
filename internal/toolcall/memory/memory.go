// Package memory implements persistent memory tools (mem_save, mem_update, mem_search,
// mem_delete, mem_get_observation, mem_stats)
package memory

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/memories"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

var RegisterTool = toolcall.RegisterTool

type (
	ToolArgs  = toolcall.ToolArgs
	ToolDef   = toolcall.ToolDef
	Primitive = toolcall.Primitive
)

func ToolArgsValue[T Primitive](args ToolArgs, key string, defaultValue T) T {
	return toolcall.ToolArgsValue(args, key, defaultValue)
}

// ─── SQLite Schema (registered via init) ──────────────────────────────────────

func init() {
	// ── Tools ──
	RegisterTool(ToolDef{
		Name: "mem_save",
		Description: `Save to persistent memory with FTS5 search.

Save important info for later retrieval. Use for:
- Recording architectural decisions
- Documenting bug fixes
- Saving discoveries, lessons, configurations

Parameters: title (required), content (required), type (decision/architecture/bugfix/pattern/config/discovery/learning).`,
		Category: "memory",
		Strict:   true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"title":   map[string]any{"type": "string", "description": "简短可搜索的标题，如'JWT认证中间件实现'"},
				"content": map[string]any{"type": "string", "description": "详细内容，建议使用 **What**/**Why**/**Where**/**Learned** 格式"},
				"type":    map[string]any{"type": "string", "description": "类型: decision, architecture, bugfix, pattern, config, discovery, learning"},
			},
			"required":             []string{"title", "content"},
			"additionalProperties": false,
		},
		Handler: handleMemSave,
	})

	RegisterTool(ToolDef{
		Name:        "mem_update",
		Description: "Update memory by ID.\n\nUpdate an existing memory. Only provided fields are modified.\nParameters: id (required), title (optional), content (optional), type (optional).",
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "integer", "description": "要更新的记忆ID（必填）"},
				"title":   map[string]any{"type": "string", "description": "新标题（可选）"},
				"content": map[string]any{"type": "string", "description": "新内容（可选）"},
				"type":    map[string]any{"type": "string", "description": "新类型（可选）"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Handler: handleMemUpdate,
	})

	RegisterTool(ToolDef{
		Name: "mem_search",
		Description: `Search memories with FTS5 full-text search.

Full-text search all persistent memories with optional type filtering and result limit.

Use when:
- Finding previous decisions
- Searching fixed bug records
- Looking up patterns or conventions
- Reviewing previous work context`,
		Category: "memory",
		Strict:   true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{"type": "string", "description": "搜索查询——自然语言或关键词（必填）"},
				"type":  map[string]any{"type": "string", "description": "按类型过滤: decision, architecture, bugfix, pattern, config, discovery, learning"},
				"limit": map[string]any{"type": "integer", "description": "最大结果数（默认10，最大50）"},
			},
			"required":             []string{"query"},
			"additionalProperties": false,
		},
		Handler: handleMemSearch,
	})

	RegisterTool(ToolDef{
		Name:        "mem_delete",
		Description: "Delete memory by ID. Irreversible.",
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "要删除的记忆ID（必填）"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Handler: handleMemDelete,
	})

	RegisterTool(ToolDef{
		Name:        "mem_get_observation",
		Description: "Get memory content by ID.\n\nRetrieve full memory content. Use when mem_search returns truncated previews and you need the complete text.",
		Category:    "memory",
		Strict:      true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id": map[string]any{"type": "integer", "description": "记忆ID（必填）"},
			},
			"required":             []string{"id"},
			"additionalProperties": false,
		},
		Handler: handleMemGetObservation,
	})

	RegisterTool(ToolDef{
		Name:        "mem_stats",
		Description: "Memory stats: total count, type distribution.",
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
		err = fmt.Errorf("title 和 content 为必填项")
		return result, warning, err
	}

	result, warning, err = memories.HandleMemSave(ctx, title, body, typ)
	return result, warning, err
}

// handleMemUpdate updates an existing memory by ID.
func handleMemUpdate(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id 为必填项")
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
		err = fmt.Errorf("query 为必填项")
		return result, warning, err
	}

	result, warning, err = memories.HandleMemSearch(ctx, query, typ, limit)
	return result, warning, err
}

// handleMemDelete deletes a memory by ID.
func handleMemDelete(ctx context.Context, args ToolArgs) (result, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id 为必填项")
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
		err = fmt.Errorf("id 为必填项")
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
