// Package memory registers memory tools (mem_save/update/search/delete/get_observation/stats)
// with the toolcall framework and parses LLM-issued tool arguments.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Layering
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// This package is a thin adapter between LLM tool calls and the core memory
// logic in internal/memories.  It owns:
//
//   - ToolDef registration (name, description, JSON Schema parameters)
//   - Argument extraction from map[string]any with defaults and validation
//   - Delegation to memories.Handle* functions
//
// The core memories package knows nothing about toolcall, JSON Schema, or
// LLMs.  It exposes pure Go functions that operate on typed parameters.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Handler Contract
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Every handler returns (result, suggest, err):
//
//	result   — LLM-visible response (Markdown, search results, error details)
//	suggest  — suggestion for LLM to do better next time (memory tools return "")
//	err      — non-nil on failure; the framework formats it for the LLM
//
// Handlers do minimal input validation (e.g. non-empty title/content, id > 0)
// then immediately delegate to memories.*.  They do not touch the database.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Tools
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// All six tools are registered in init() via toolcall.RegisterTool.
// Each tool's JSON Schema specifies required fields and
// additionalProperties: false, enabling strict LLM parsing.
//
// ── mem_save ──────────────────────────────────────────────────────────────────
//
//	When  LLM needs to persist important information for future cross-session
//	      recall (architecture decisions, bug fixes, patterns, config changes).
//	Why   LLMs are stateless — without persistence, accumulated knowledge is
//	      lost between sessions, forcing redundant re-explanation.
//	How   LLM calls mem_save(title, content, type?) with a searchable title,
//	      structured content (e.g. **What**/**Why**/**Where**/**Learned**), and
//	      an optional category type.
//	What  Enables cross-session memory continuity, knowledge accumulation,
//	      and context-aware recall via FTS5 full-text search.
//
// ── mem_update ────────────────────────────────────────────────────────────────
//
//	When  An existing memory needs correction, augmentation, or re-categorization
//	      without losing its ID and history.
//	Why   Information evolves; patch semantics avoid delete+recreate churn and
//	      keep search indices consistent.
//	How   LLM provides the memory ID and any subset of fields (title, content,
//	      type) to update. Unspecified fields are left unchanged.
//	What  Keeps memories accurate and current with minimal effort and no
//	      duplication.
//
// ── mem_search ────────────────────────────────────────────────────────────────
//
//	When  LLM needs to recall previously saved information before making a
//	      decision, fixing a bug, or continuing prior work.
//	Why   The system prompt cannot hold all historical context; FTS5 enables
//	      fast semantic retrieval of relevant past entries.
//	How   LLM provides a natural-language query (or keywords) and optional
//	      type/limit filters. Results are returned as truncated previews.
//	What  Fast, targeted retrieval of past decisions, patterns, bug fixes,
//	      and context — reducing redundant work and improving consistency.
//
// ── mem_delete ────────────────────────────────────────────────────────────────
//
//	When  A memory is obsolete, incorrect, or should be removed for any reason.
//	Why   Stale information pollutes search results and wastes LLM context
//	      window space.
//	How   LLM provides the memory ID. Deletion is irreversible.
//	What  Keeps the memory store clean, relevant, and efficient.
//
// ── mem_get_observation ───────────────────────────────────────────────────────
//
//	When  mem_search returns a truncated preview and the LLM needs the full
//	      memory content for detailed analysis.
//	Why   Search results are truncated to save tokens; full-content retrieval
//	      is needed only for high-value matches.
//	How   LLM provides the memory ID (typically obtained from a prior
//	      mem_search result).
//	What  On-demand access to complete memory content without bloating every
//	      search response.
//
// ── mem_stats ─────────────────────────────────────────────────────────────────
//
//	When  LLM wants a quick overview of the memory system — total count,
//	      type distribution, storage health.
//	Why   Understanding what is stored helps the LLM decide whether to search,
//	      save, or prune.
//	How   LLM calls mem_stats() with no arguments.
//	What  Instant system health check and content inventory to guide memory
//	      strategy.
package memory
