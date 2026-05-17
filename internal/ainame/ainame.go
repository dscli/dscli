// Package ainame manages AI persona names for system prompt injection.
//
// 32 names (15 bird + 17 frog, Dyson classification) are seeded into SQLite
// on first database init. Each session gets a randomly assigned name on first
// use, persisted in session_names. The assignment is permanent — INSERT OR
// IGNORE semantics prevent reassignment.
//
// sessionID == 0 or DB errors fall back to "nobody", the every-programmer.
package ainame

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"strings"

	"gitcode.com/dscli/dscli/internal/sqlite"
)

// NameConfig holds the AI persona data for prompt injection.
type NameConfig struct {
	NameCN        string // e.g. "牛顿", "无名"
	NameEN        string // e.g. "Newton", "nobody"
	PersonalityEN string // e.g. "steady, forceful, vain"
	DescEN        string // English description for prompt injection
	BirdFrog      string // "bird" | "frog"
	Email         string // lower(name_en)@dscli.io
}

// nobodyCfg is the fallback for sessionID == 0 or DB errors.
var nobodyCfg = NameConfig{
	NameCN:        "无名",
	NameEN:        "nobody",
	PersonalityEN: "invisible, nameless, ubiquitous",
	DescEN:        "Like a null pointer in C — the silent foundation that countless programmers build upon. You are the every-programmer: unseen, indispensable. Your code runs in the background of the digital world, unnoticed but essential. You do not seek recognition; you seek correctness.",
	BirdFrog:      "frog",
	Email:         "nobody@dscli.io",
}

func init() {
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS ai_names (
			id              INTEGER PRIMARY KEY,
			name_cn         TEXT NOT NULL,
			name_en         TEXT NOT NULL,
			bird_frog       TEXT NOT NULL DEFAULT 'frog',
			personality_cn  TEXT NOT NULL,
			personality_en  TEXT NOT NULL,
			desc_cn         TEXT NOT NULL,
			desc_en         TEXT NOT NULL,
			email           TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS session_names (
			session_id  INTEGER PRIMARY KEY,
			name_id     INTEGER NOT NULL DEFAULT 0,
			assigned_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id),
			FOREIGN KEY (name_id) REFERENCES ai_names(id)
		)`,
	)

	sqlite.RegisterPostInitHook(func(db *sql.DB) error {
		return seedNames(db)
	})
}

// seedNames inserts the 32 names into ai_names using INSERT OR IGNORE.
func seedNames(db *sql.DB) error {
	for _, n := range namesData {
		email := n.Email
		if email == "" {
			email = strings.ToLower(n.NameEN) + "@dscli.io"
		}
		_, err := db.Exec(
			`INSERT OR IGNORE INTO ai_names
			 (id, name_cn, name_en, bird_frog, personality_cn, personality_en, desc_cn, desc_en, email)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			n.ID, n.NameCN, n.NameEN, n.BirdFrog,
			n.PersonalityCN, n.PersonalityEN, n.DescCN, n.DescEN,
			email,
		)
		if err != nil {
			return fmt.Errorf("seed ai_names id=%d: %w", n.ID, err)
		}
	}
	return nil
}

// LoadOrAssign returns the AI name for a session.
//
// sessionID == 0 → returns nobody (no session initialized).
// sessionID > 0  → returns previously assigned name, or randomly assigns one.
// DB errors       → falls back to nobody.
func LoadOrAssign(sessionID int64) *NameConfig {
	if sessionID == 0 {
		return &nobodyCfg
	}

	db, err := sqlite.OpenDB()
	if err != nil {
		return &nobodyCfg
	}
	defer db.Close()

	// 1. Check existing assignment
	var nameID int64
	err = db.QueryRow(
		"SELECT name_id FROM session_names WHERE session_id = ?", sessionID,
	).Scan(&nameID)

	if err == sql.ErrNoRows {
		// 2. No row yet — randomly assign
		nameID = int64(rand.IntN(len(namesData))) + 1 // IDs are 1-32
		_, err = db.Exec(
			"INSERT INTO session_names (session_id, name_id) VALUES (?, ?)",
			sessionID, nameID,
		)
		if err != nil {
			// Race or FK violation — re-read; if still absent, fall through to nobody
			nameID = 0
			_ = db.QueryRow(
				"SELECT name_id FROM session_names WHERE session_id = ?", sessionID,
			).Scan(&nameID)
		}
	} else if err != nil {
		return &nobodyCfg
	}
	if nameID == 0 {
		return &nobodyCfg
	}

	// 3. Read name data
	var cfg NameConfig
	err = db.QueryRow(
		`SELECT name_cn, name_en, personality_en, desc_en, bird_frog, email
		 FROM ai_names WHERE id = ?`, nameID,
	).Scan(&cfg.NameCN, &cfg.NameEN, &cfg.PersonalityEN, &cfg.DescEN, &cfg.BirdFrog, &cfg.Email)
	if err != nil {
		return &nobodyCfg
	}

	return &cfg
}
