// Package memories implements persistent memory tools backed by SQLite FTS5.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Architecture
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// The memory system is split into two layers:
//
//   - internal/memories  — Core domain logic (this package)
//   - internal/toolcall/memory — LLM tool registration & argument parsing
//
// This separation keeps the memory logic independent of the toolcall framework.
// The toolcall layer parses raw ToolArgs (map[string]any) into typed parameters,
// then delegates to the handler functions exported by this package. Each handler
// returns (result, suggest, error) — LLM-visible response, improvement suggestion, and error.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Schema Management
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// SQLite schemas are registered via init() using the sqlite package's
// declarative registration API:
//
//	sqlite.RegisterTableSchema(...)    — CREATE TABLE, CREATE VIRTUAL TABLE
//	sqlite.RegisterIndexSchema(...)    — CREATE INDEX
//
// This pattern ensures all schema registrations are collected before
// sqlite.OpenDB() lazily initializes the database once via sync.Once.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Chinese Tokenization (internal/tokenizer, github.com/go-ego/gse)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// memories_fts is a standalone FTS5 virtual table (no content= option).
// FTS5 sync is managed explicitly in Go:
//
//	HandleMemSave    → insertFTS()     after INSERT
//	HandleMemUpdate  → deleteFTS() + insertFTS() after UPDATE
//	HandleMemDelete  → deleteFTS()     after DELETE
//
// Chinese text is tokenized via internal/tokenizer (gse CutSearch mode),
// which produces both compound words and their sub-words for better recall.
//
// Tokenized words are space-joined and inserted into the FTS5 index.  FTS5's
// unicode61 tokenizer splits on spaces (keeping CJK runs intact), so each gse
// word becomes an independent FTS5 token.
//
// Search queries go through the identical pipeline — tokenizer.SanitizeFTS
// ensures the same token boundaries on both sides.  Each token is wrapped in
// double quotes for FTS5 phrase matching, making the implicit AND semantics
// explicit.
//
// The gse Segmenter is initialized once (sync.Once) inside the tokenizer package.
// Dictionary loading happens lazily on first Tokenize() / SanitizeFTS() call.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Data Model
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
//	memoryRow     — a single memory record (id, title, content, type, timestamps)
//	searchRow     — memoryRow + FTS rank score
//
// Types supported: decision, architecture, bugfix, pattern, config, discovery,
// learning, manual (default).
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Handlers
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
//	HandleMemSave            — Insert a new memory with title, content, type
//	HandleMemUpdate          — Update fields of an existing memory by ID
//	HandleMemSearch          — FTS5 search with type filter and result limit
//	HandleMemDelete          — Delete a memory by ID (verifies existence first)
//	HandleMemGetObservation  — Retrieve full content by ID (vs. search's 300-char preview)
//	HandleMemStats           — Aggregate stats: total count + type distribution + latest entry
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Testing
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// In test mode (context.IsTesting() returning true), sqlite uses a temporary
// database in os.TempDir(), isolated from production data. Tests use
// sqlite.SetDBPath() + t.TempDir() for per-test isolation.
//
// Pure unit tests cover tokenize, sanitizeFTS, truncate without a DB.
// Integration tests cover the full lifecycle: save → search → update → search → delete.
// CJK-specific tests verify Chinese word, sub-word, and mixed-language search
// using gse CutSearch tokenization.
package memories