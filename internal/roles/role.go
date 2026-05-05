// Package roles manages role-to-skills/tools/prompt mappings stored in SQLite.
//
// Each role (dev/expert/review/writer/editor) can have per-session configuration.
// The table is keyed by (role, session_id), not (role, project_path). This is
// intentional: session_id is a stable identifier that survives project relocation.
// When a user copies a project to a new directory, they only need to update
// sessions.project_path — role_configs follows automatically.
//
// Fallback: when no row exists for a role+session, the system uses hardcoded
// defaults: dev gets all skills+tools, expert/review get none.
//
// API conventions:
//   - "all"  → nil slice (no filtering, include everything)
//   - ""     → empty slice (explicitly nothing)
//   - "a,b"  → ["a","b"] slice (filter to these names)
//
// All exported functions take int64 sessionID, not string projectPath.
package roles

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

// RoleConfig maps a role to its skills, tools, and prompt template.
type RoleConfig struct {
	ID        int64
	Role      string // e.g. "dev", "expert", "review", "writer", "editor"
	Skills    string // "all", "", or comma-separated skill names
	Tools     string // "all", "", or comma-separated tool names
	Prompt    string // prompt template name; empty means use role name
	SessionID int64  // FK to sessions.id
	CreatedAt time.Time
	UpdatedAt time.Time
}

func init() {
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS role_configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL,
			skills TEXT NOT NULL DEFAULT 'all',
			tools TEXT NOT NULL DEFAULT 'all',
			prompt TEXT NOT NULL DEFAULT '',
			session_id INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(role, session_id)
		)`,
	)

	// Migration runs as post-init hook so sessions table exists when we join on it.
	sqlite.RegisterPostInitHook(migrateRoleConfigs)
}

// migrateRoleConfigs detects old schema (project_path column) and migrates to
// session_id. For new installs it just ensures the index exists.
func migrateRoleConfigs(db *sql.DB) error {
	// Check if migration is needed.
	rows, err := db.Query(`PRAGMA table_info(role_configs)`)
	if err != nil {
		// Table might not exist — fresh DB, just ensure index exists.
		_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_role_configs_session ON role_configs(session_id)`)
		return nil
	}

	var hasProjectPath bool
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			continue
		}
		if name == "project_path" {
			hasProjectPath = true
			break
		}
	}
	rows.Close() // must close before DDL to avoid SQLITE_BUSY

	if !hasProjectPath {
		// Already migrated or new install — just ensure index.
		_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_role_configs_session ON role_configs(session_id)`)
		return nil
	}

	// Step 1: Create new table.
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS role_configs_new (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			role TEXT NOT NULL,
			skills TEXT NOT NULL DEFAULT 'all',
			tools TEXT NOT NULL DEFAULT 'all',
			prompt TEXT NOT NULL DEFAULT '',
			session_id INTEGER NOT NULL DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(role, session_id)
		)`)
	if err != nil {
		return fmt.Errorf("roles: 创建新表失败: %w", err)
	}

	// Step 2: Migrate data via sessions join.
	_, err = db.Exec(`
		INSERT OR IGNORE INTO role_configs_new
			(role, skills, tools, prompt, session_id, created_at, updated_at)
		SELECT o.role, o.skills, o.tools, o.prompt,
			COALESCE(s.id, 0), o.created_at, o.updated_at
		FROM role_configs o
		LEFT JOIN sessions s ON s.project_path = o.project_path`)
	if err != nil {
		return fmt.Errorf("roles: 迁移数据失败: %w", err)
	}

	// Step 3: Swap tables.
	_, err = db.Exec(`DROP TABLE role_configs`)
	if err != nil {
		return fmt.Errorf("roles: 删除旧表失败: %w", err)
	}
	_, err = db.Exec(`ALTER TABLE role_configs_new RENAME TO role_configs`)
	if err != nil {
		return fmt.Errorf("roles: 重命名新表失败: %w", err)
	}

	// Step 4: Create index.
	_, _ = db.Exec(`CREATE INDEX IF NOT EXISTS idx_role_configs_session ON role_configs(session_id)`)
	return nil
}

// GetRoleConfig retrieves the role config for a given role and session.
// Returns nil if no config exists (caller should fall back to hardcoded defaults).
func GetRoleConfig(role string, sessionID int64) (*RoleConfig, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	cfg := &RoleConfig{}
	err = db.QueryRow(
		`SELECT id, role, skills, tools, prompt, session_id, created_at, updated_at
		 FROM role_configs WHERE role = ? AND session_id = ?`,
		role, sessionID,
	).Scan(&cfg.ID, &cfg.Role, &cfg.Skills, &cfg.Tools, &cfg.Prompt,
		&cfg.SessionID, &cfg.CreatedAt, &cfg.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询角色配置失败: %w", err)
	}
	return cfg, nil
}

// ListRoleConfigs returns all role configs for a given session.
func ListRoleConfigs(sessionID int64) ([]RoleConfig, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.Query(
		`SELECT id, role, skills, tools, prompt, session_id, created_at, updated_at
		 FROM role_configs WHERE session_id = ? ORDER BY role`,
		sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("查询角色配置列表失败: %w", err)
	}
	defer rows.Close()

	var configs []RoleConfig
	for rows.Next() {
		var cfg RoleConfig
		if err := rows.Scan(&cfg.ID, &cfg.Role, &cfg.Skills, &cfg.Tools, &cfg.Prompt,
			&cfg.SessionID, &cfg.CreatedAt, &cfg.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描角色配置失败: %w", err)
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// UpsertRoleConfig inserts or updates a role config.
func UpsertRoleConfig(role string, sessionID int64, skills, tools, prompt string) error {
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var id int64
	err = db.QueryRow(
		`SELECT id FROM role_configs WHERE role = ? AND session_id = ?`,
		role, sessionID,
	).Scan(&id)

	if err == sql.ErrNoRows {
		_, err = db.Exec(
			`INSERT INTO role_configs (role, skills, tools, prompt, session_id)
			 VALUES (?, ?, ?, ?, ?)`,
			role, skills, tools, prompt, sessionID,
		)
		if err != nil {
			return fmt.Errorf("插入角色配置失败: %w", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("查询角色配置失败: %w", err)
	}

	_, err = db.Exec(
		`UPDATE role_configs
		 SET skills = CASE WHEN ? != '' THEN ? ELSE skills END,
		     tools = CASE WHEN ? != '' THEN ? ELSE tools END,
		     prompt = CASE WHEN ? != '' THEN ? ELSE prompt END,
		     updated_at = CURRENT_TIMESTAMP
		 WHERE id = ?`,
		skills, skills,
		tools, tools,
		prompt, prompt,
		id,
	)
	if err != nil {
		return fmt.Errorf("更新角色配置失败: %w", err)
	}
	return nil
}

// DeleteRoleConfig deletes a role config.
func DeleteRoleConfig(role string, sessionID int64) error {
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(
		`DELETE FROM role_configs WHERE role = ? AND session_id = ?`,
		role, sessionID,
	)
	if err != nil {
		return fmt.Errorf("删除角色配置失败: %w", err)
	}
	return nil
}

// ParseSkillsList parses the skills field.
// Returns nil for "all" (no filtering), empty slice for "" (nothing), or names.
func ParseSkillsList(skills string) []string {
	return parseList(skills)
}

// ParseToolsList parses the tools field.
// Returns nil for "all" (no filtering), empty slice for "" (nothing), or names.
func ParseToolsList(tools string) []string {
	return parseList(tools)
}

func parseList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	if s == "all" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
