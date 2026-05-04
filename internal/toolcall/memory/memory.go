// Package memory implements persistent memory tools (mem_save, mem_update, mem_search,
// mem_delete, mem_get_observation, mem_stats) 
//
package memory

import (
	"context"
	"fmt"

	"gitcode.com/dscli/dscli/internal/memories"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

var (
	RegisterTool = toolcall.RegisterTool
)

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
		Name:        "mem_save",
		Description: `🗄️ 将重要信息保存到持久记忆中。使用FTS5全文搜索，支持后续检索。

何时使用：
- 保存架构决策、设计思路
- 记录bug修复过程和解决方案
- 保存重要发现、教训和经验
- 记录配置变更、环境设置

参数：
- title: 简洁、可搜索的标题（必填）
- content: 详细内容，建议使用结构化格式（必填）
- type: 类型，如 decision, architecture, bugfix, pattern, config, discovery, learning（默认 manual）`,
		Category:    "memory",
		Strict:      true,
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
		Description: "✏️ 通过ID更新已有记忆。只更新提供的字段。",
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
		Name:        "mem_search",
		Description: `🔍 全文搜索所有持久记忆。支持按类型过滤和限制结果数量。使用FTS5搜索引擎。

使用场景：
- 查找之前做过的决策
- 搜索已修复的bug记录
- 查找特定模式或约定的使用
- 回顾之前的工作上下文`,
		Category:    "memory",
		Strict:      true,
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
		Description: "🗑️ 按ID删除记忆。删除操作不可逆。",
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
		Description: "📖 按ID获取记忆完整内容（mem_search 返回截断预览，用此工具查看全文）。",
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
		Description: "📊 记忆系统统计：总数、类型分布。",
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
func handleMemSave(ctx context.Context, args ToolArgs) (result string, warning string, err error) {
	title := ToolArgsValue(args, "title", "")
	body := ToolArgsValue(args, "content", "")
	typ := ToolArgsValue(args, "type", "manual")

	if title == "" || body == "" {
		err = fmt.Errorf("title 和 content 为必填项")
		return
	}

	result, warning, err = memories.HandleMemSave(ctx, title, body, typ)
	return
}

// handleMemUpdate updates an existing memory by ID.
func handleMemUpdate(ctx context.Context, args ToolArgs) (result string, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id 为必填项")
		return
	}

	// Build update with provided fields only
	title := ToolArgsValue(args, "title", "")
	body := ToolArgsValue(args, "content", "")
	typ := ToolArgsValue(args, "type", "")
	result, warning, err = memories.HandleMemUpdate(ctx, id, title, body, typ)
	return
}

// handleMemSearch searches memories using FTS5 full-text search.
func handleMemSearch(ctx context.Context, args ToolArgs) (result string, warning string, err error) {
	query := ToolArgsValue(args, "query", "")
	typ := ToolArgsValue(args, "type", "")
	limit := ToolArgsValue(args, "limit", 10)
	limit = min(limit, 50)
	if limit <= 0 {
		limit = 10
	}

	if query == "" {
		err = fmt.Errorf("query 为必填项")
		return
	}

	result, warning, err = memories.HandleMemSearch(ctx, query, typ, limit)
	return
}

// handleMemDelete deletes a memory by ID.
func handleMemDelete(ctx context.Context, args ToolArgs) (result string, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id 为必填项")
		return
	}
	result, warning, err = memories.HandleMemDelete(ctx, id)
	return
}

// handleMemGetObservation retrieves full memory content by ID.
// Unlike mem_search which returns truncated previews, this returns the complete content.
func handleMemGetObservation(ctx context.Context, args ToolArgs) (result string, warning string, err error) {
	id := ToolArgsValue(args, "id", int64(0))
	if id == 0 {
		err = fmt.Errorf("id 为必填项")
		return
	}
	result, warning, err = memories.HandleMemGetObservation(ctx, id)
	return
}

// handleMemStats returns memory system statistics.
func handleMemStats(ctx context.Context, _ ToolArgs) (result string, warning string, err error) {
	result, warning, err = memories.HandleMemStats(ctx)
	return
}
