// Package roles manages role-to-skills/tools/prompt mappings stored in SQLite.
//
// Each role (dev/expert/review/test) can have per-session configuration.
// The table is keyed by (role, session_id), not (role, project_path). This is
// intentional: session_id is a stable identifier that survives project relocation.
// When a user copies a project to a new directory, they only need to update
// sessions.project_path — role_configs follows automatically.
//
// Fallback: when no row exists for a role+session, the system uses hardcoded
// defaults: dev gets all skills+tools, expert/review/test get none.
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
	"sync"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/nanjj/clog"

	"github.com/dscli/dscli/internal/sqlite"
)

// RoleConfig maps a role to its skills, tools, and prompt template.
type RoleConfig struct {
	ID        int64
	Role      string // e.g. "dev", "expert", "review"
	Skills    string // "all", "", or comma-separated skill names
	Tools     string // "all", "", or comma-separated tool names
	Prompt    string // prompt template name; empty means use role name
	SessionID int64  // FK to sessions.id
	CreatedAt time.Time
	UpdatedAt time.Time
}

var (
	roleCache   map[string]*RoleConfig // role name → config, nil until loaded
	roleCacheMu sync.RWMutex
)

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
}

// GetRoleConfig retrieves the role config for a given role and session.
// Uses an in-memory cache loaded once per process lifetime;
// falls back to direct DB query when cache loading fails.
func GetRoleConfig(ctx context.Context, role string, sessionID int64) (*RoleConfig, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "GetRoleConfig")
	defer span.Finish()
	// Fast path: read from cache (RLock only).
	roleCacheMu.RLock()
	if roleCache != nil {
		cfg := roleCache[role]
		roleCacheMu.RUnlock()
		return cfg, nil
	}
	roleCacheMu.RUnlock()

	// Slow path: load cache (one-time, requires write lock).
	roleCacheMu.Lock()
	if roleCache != nil {
		// Another goroutine loaded the cache while we waited.
		cfg := roleCache[role]
		roleCacheMu.Unlock()
		return cfg, nil
	}

	configs, err := ListRoleConfigs(ctx, sessionID)
	if err == nil {
		m := make(map[string]*RoleConfig, len(configs))
		for i := range configs {
			m[configs[i].Role] = &configs[i]
		}
		roleCache = m
		cfg := m[role]
		roleCacheMu.Unlock()
		return cfg, nil
	}
	roleCacheMu.Unlock()

	// Fallback: direct DB query.
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)

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
func ListRoleConfigs(ctx context.Context, sessionID int64) ([]RoleConfig, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "ListRoleConfigs")
	defer span.Finish()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)

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

// ListRoleConfigsByPrompt returns all role configs whose prompt field
// references the given prompt name.
// A positive sessionID restricts the scan to a single session; 0 scans
// all sessions. Used by `prompt remove` to find dangling references
// after a prompt file is deleted.
func ListRoleConfigsByPrompt(ctx context.Context, promptName string, sessionID int64) ([]RoleConfig, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "ListRoleConfigsByPrompt")
	defer span.Finish()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)

	query := `SELECT id, role, skills, tools, prompt, session_id, created_at, updated_at
		 FROM role_configs WHERE prompt = ?`
	args := []any{promptName}
	if sessionID > 0 {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	query += ` ORDER BY role`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("查询引用提示词的配置失败: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历角色配置失败: %w", err)
	}
	return configs, nil
}

// invalidateRoleCache clears the in-memory role config cache.
// Call after any write to role_configs to ensure subsequent reads
// see the latest data.
func invalidateRoleCache() {
	roleCacheMu.Lock()
	roleCache = nil
	roleCacheMu.Unlock()
}

// UpsertRoleConfig inserts or updates a role config.
func UpsertRoleConfig(ctx context.Context, role string, sessionID int64, skills, tools, prompt string) error {
	span, ctx := clog.StartSpanFromContext(ctx, "UpsertRoleConfig")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)

	var id int64
	err = db.QueryRow(
		`SELECT id FROM role_configs WHERE role = ? AND session_id = ?`,
		role, sessionID,
	).Scan(&id)

	if err == sql.ErrNoRows {
		// INSERT 语义与 UPDATE 保持一致："" 表示未指定。
		// skills/tools 未指定时回落 "all"（与表 DEFAULT 'all' 及文档承诺
		// "新建时未指定的标志默认为 all" 一致）。此前直接写入 '' 会把
		// tools='' 落库 → ParseToolsList 返回空 → GetAllTools 无工具可给模型，
		// 曾导致角色工具调用完全失效（2026-07-31 事故，见 role_cmd.go 帮助）。
		// prompt 的 '' 是合法语义（表示使用角色同名默认模板），保持原样。
		if skills == "" {
			skills = "all"
		}
		if tools == "" {
			tools = "all"
		}
		_, err = db.Exec(
			`INSERT INTO role_configs (role, skills, tools, prompt, session_id)
			 VALUES (?, ?, ?, ?, ?)`,
			role, skills, tools, prompt, sessionID,
		)
		if err != nil {
			return fmt.Errorf("插入角色配置失败: %w", err)
		}
		invalidateRoleCache()
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
	invalidateRoleCache()
	return nil
}

// DeleteRoleConfig deletes a role config.
func DeleteRoleConfig(ctx context.Context, role string, sessionID int64) error {
	span, ctx := clog.StartSpanFromContext(ctx, "DeleteRoleConfig")
	defer span.Finish()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)

	_, err = db.Exec(
		`DELETE FROM role_configs WHERE role = ? AND session_id = ?`,
		role, sessionID,
	)
	if err != nil {
		return fmt.Errorf("删除角色配置失败: %w", err)
	}
	invalidateRoleCache()
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
