package ainame

import (
	"testing"

	"gitcode.com/dscli/dscli/internal/session"
)

func TestLoadOrAssign_Nobody(t *testing.T) {
	cfg := LoadOrAssign(0)
	if cfg == nil {
		t.Fatal("LoadOrAssign(0) returned nil")
	}
	if cfg.NameEN != "nobody" {
		t.Fatalf("expected nobody, got %s", cfg.NameEN)
	}
	if cfg.NameCN != "无名" {
		t.Fatalf("expected 无名, got %s", cfg.NameCN)
	}
	if cfg.BirdFrog != "frog" {
		t.Fatalf("expected frog, got %s", cfg.BirdFrog)
	}
}

func TestLoadOrAssign_AssignAndPersist(t *testing.T) {
	ctx := t.Context()
	sessionID := session.GetCurrentSessionID(ctx)
	if sessionID == 0 {
		t.Fatal("session.GetCurrentSessionID returned 0")
	}

	// First assignment
	cfg := LoadOrAssign(sessionID)
	if cfg == nil {
		t.Fatal("LoadOrAssign returned nil")
	}
	if cfg.NameCN == "" || cfg.NameEN == "" || cfg.PersonalityEN == "" || cfg.DescEN == "" {
		t.Fatal("NameConfig fields should not be empty")
	}
	if cfg.BirdFrog != "bird" && cfg.BirdFrog != "frog" {
		t.Fatalf("unexpected bird_frog value: %s", cfg.BirdFrog)
	}

	// Second call should return same name (idempotent)
	cfg2 := LoadOrAssign(sessionID)
	if cfg2.NameEN != cfg.NameEN {
		t.Fatalf("assignment changed: %s → %s", cfg.NameEN, cfg2.NameEN)
	}
}
