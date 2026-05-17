// Package ainame assigns a unique AI thinker persona to each session.
//
// Architecture:
//
//   - 32 hardcoded names (15 bird + 17 frog), classified after Freeman Dyson's
//     "Birds and Frogs" essay. Each name carries an English personality sketch
//     and a one-sentence cognitive style for prompt injection.
//
//   - Seeded into ai_names table via INSERT OR IGNORE on first DB init.
//
//   - LoadOrAssign(sessionID) → *NameConfig
//
//   - sessionID == 0 → "nobody" (the every-programmer)
//
//   - Existing assignment → read from session_names + ai_names
//
//   - No assignment → rand pick, INSERT, then read
//
//   - DB errors → "nobody" fallback
//
//   - Nobody is id=9, permanent fallback: unseen, indispensable, seeks correctness.
//
// Prompt integration:
//
//	prompt.go calls LoadOrAssign(sessionID) and injects NameEN, PersonalityEN,
//	DescEN into the "Persona" section of system prompt templates (dev, expert,
//	review). No conditional logic — nobody guarantees a valid value always.
package ainame
