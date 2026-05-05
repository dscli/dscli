// Package sqlite provides a declarative, lazy-initialized SQLite connection with
// built-in test isolation and schema migration support.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Architecture
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Packages register schemas via init() using four collectors.  All registrations
// happen before OpenDB() is called; a sync.Once gate ensures the database is
// initialized exactly once, regardless of concurrent callers.
//
//	sqlite.RegisterTableSchema(...)     — CREATE TABLE / CREATE VIRTUAL TABLE
//	sqlite.RegisterIndexSchema(...)     — CREATE INDEX
//	sqlite.RegisterUpgradeSchema(...)   — ALTER TABLE ADD COLUMN / migration SQL
//	sqlite.RegisterPostInitHook(...)    — func(*sql.DB) error callbacks
//
// Execution order inside initDatabase():
//
//  1. Table schemas   — fatal on error
//  2. Index schemas   — fatal on error
//  3. Upgrade schemas — best-effort (errors silently ignored)
//  4. Post-init hooks — best-effort (errors logged to Debug)
//
// This order matters: indexes run before upgrades.  If an upgrade adds a column,
// any index on that column MUST be registered in RegisterUpgradeSchema after the
// ALTER TABLE — never in RegisterIndexSchema.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Database Path Selection (Test Isolation)
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// choosing from two strategies in priority order:
//
//	context.IsTesting() true  →  /tmp/dscli-test-<binary>-<pid>.db
//	Otherwise                  →  ~/.dscli/sqlite.db  (production)
//
// context.IsTesting() checks whether os.Args[0] ends with ".test" — the suffix
// that 'go test' uses for compiled test binaries.  This means any test that
// imports the sqlite package automatically gets a temp database, never touching
// production data.  No setup required.
//
// Per-test customization: tests that need fully independent databases can call
// allowing re-initialization with the new path.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Upgrade Schema: Adding & Removing Columns
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// SQLite's ALTER TABLE is limited.  This package embraces a "best-effort
// upgrade" pattern:
//
// Adding a column:
//
//	sqlite.RegisterUpgradeSchema(
//	    `ALTER TABLE memories ADD COLUMN session_id INTEGER NOT NULL DEFAULT 0`,
//	    `CREATE INDEX IF NOT EXISTS idx_memories_session_id ON memories(session_id)`,
//	)
//
// The ALTER TABLE fails silently if the column already exists (duplicate column
// name).  The CREATE INDEX uses IF NOT EXISTS for idempotency.  Together they
// handle three states correctly: fresh DB (column exists, ALTER skipped),
// migrated DB (column+index exist, both skipped), and legacy DB (column added,
// index created).
//
// Removing a column — SQLite ≥3.35.0 supports DROP COLUMN:
//
//	sqlite.RegisterUpgradeSchema(
//	    `ALTER TABLE foo DROP COLUMN deprecated_field`,
//	)
//
// For older SQLite or complex migrations, use RegisterPostInitHook to run
// arbitrary Go logic (e.g. recreate-table-and-copy pattern).
//
// ⚠️ Critical rule: any index on a column added by an upgrade must live in
// RegisterUpgradeSchema, after the ALTER TABLE that creates the column.
// RegisterIndexSchema runs first — if the column doesn't exist yet, the index
// creation fails fatally and initDatabase returns an error.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// FTS5 Full-Text Search
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// FTS5 virtual tables are created via RegisterTableSchema, same as regular
// tables.  The memories package demonstrates the canonical pattern:
//
//	sqlite.RegisterTableSchema(
//	    `CREATE TABLE IF NOT EXISTS memories (...)` ,
//	    `CREATE VIRTUAL TABLE IF NOT EXISTS memories_fts USING fts5(
//	        title, content, type
//	    )`,
//	)
//
// Standalone FTS5 (no content= option) requires explicit sync — the application
// must insert/update/delete FTS rows itself.  The memories package does this
// with insertFTS() / deleteFTS() helpers called after each CRUD operation.
//
// Chinese text requires pre-tokenization before FTS indexing.  The tokenizer
// package (internal/tokenizer) uses gse CutSearch to split CJK text into
// space-separated words, which FTS5's unicode61 tokenizer then indexes as
// independent tokens.  This is handled at the application layer — the sqlite
// package is unaware of tokenization.
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Connection Parameters
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// Open() appends pragmas to the DSN:
//
//	_journal=WAL     — write-ahead logging, better concurrent reads
//	_timeout=5000    — 5-second busy timeout before SQLITE_BUSY
//	_fk=1            — enforce foreign key constraints
//
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
// Consumer Example
// ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
//
// A typical consumer package:
//
//	func init() {
//	    sqlite.RegisterTableSchema(`CREATE TABLE IF NOT EXISTS foo (...)`)
//	    sqlite.RegisterIndexSchema(`CREATE INDEX IF NOT EXISTS idx_foo_bar ON foo(bar)`)
//	    sqlite.RegisterUpgradeSchema(`ALTER TABLE foo ADD COLUMN baz TEXT`)
//	}
//
//	func doWork() error {
//	    db, err := sqlite.OpenDB()
//	    if err != nil {
//	        return err
//	    }
//	    defer db.Close()
//	    // ... use db ...
//	}
package sqlite
