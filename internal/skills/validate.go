package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/goccy/go-yaml"
)

// ValidateSkillDir validates a skill directory against the Agent Skills spec.
// Returns a list of human-readable validation errors, or empty slice if valid.
//
// Checks performed (aligned with skills-ref validate command):
//   - Path exists and is a directory
//   - SKILL.md exists (prefers SKILL.md, accepts skill.md)
//   - YAML frontmatter is valid
//   - Frontmatter is a mapping (not a list or scalar)
//   - Required fields: name, description
//   - Name format: lowercase, Unicode letters allowed, no leading/trailing
//     hyphens, no consecutive hyphens, max 64 chars
//   - Directory name matches skill name (NFKC normalized)
//   - Description: non-empty, max 1024 chars
//   - Compatibility: if present, must be string, max 500 chars
//   - No unexpected fields (spec allows 6 fields + dscli extensions: keywords, auto_inject, author)
func ValidateSkillDir(dir string) []string {
	var errors []string

	// 1. Path exists and is a directory
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{fmt.Sprintf("Path does not exist: %s", dir)}
		}
		return []string{fmt.Sprintf("Cannot access path: %s: %v", dir, err)}
	}
	if !info.IsDir() {
		return []string{fmt.Sprintf("Not a directory: %s", dir)}
	}

	// 2. SKILL.md exists (prefer SKILL.md, fall back to skill.md)
	skillMDPath := findSkillMD(dir)
	if skillMDPath == "" {
		return []string{"Missing required file: SKILL.md"}
	}

	// 3-4. Parse raw YAML frontmatter
	fm, err := parseFrontmatterRaw(skillMDPath)
	if err != nil {
		return []string{fmt.Sprintf("Failed to parse SKILL.md frontmatter: %v", err)}
	}

	// Frontmatter must be a mapping
	if fm == nil {
		return []string{"YAML frontmatter is not a mapping (expected key-value pairs)"}
	}

	// 5. Check required fields
	name, _ := getStringField(fm, "name")
	desc, _ := getStringField(fm, "description")

	if name == "" {
		errors = append(errors, "Missing required field: name")
	}
	if desc == "" {
		errors = append(errors, "Missing required field: description")
	}

	// If name is missing, we can't validate name-related rules further
	if name != "" {
		// 6. Validate name format (NFKC normalized)
		normalized := norm.NFKC.String(name)
		errors = append(errors, validateName(normalized)...)

		// 7. Directory name must match skill name (NFKC normalized)
		dirName := filepath.Base(dir)
		if norm.NFKC.String(dirName) != normalized {
			errors = append(errors, fmt.Sprintf(
				"Directory name %q must match skill name %q", dirName, name))
		}

		// 8. Description length ≤ 1024 chars
		if len([]rune(desc)) > 1024 {
			errors = append(errors, fmt.Sprintf(
				"Description exceeds 1024 character limit (%d chars)", len([]rune(desc))))
		}

		// 9. Compatibility validation (if present)
		if compat, ok := fm["compatibility"]; ok {
			s, isStr := compat.(string)
			if !isStr {
				errors = append(errors, "compatibility must be a string")
			} else if len([]rune(s)) > 500 {
				errors = append(errors, fmt.Sprintf(
					"compatibility exceeds 500 character limit (%d chars)", len([]rune(s))))
			}
		}

		// 10. Unexpected fields check
		//    spec fields: name, description, license, allowed-tools, metadata, compatibility
		//    dscli extensions: keywords, auto_inject, author
		allowedFields := map[string]bool{
			"name": true, "description": true, "license": true,
			"allowed-tools": true, "metadata": true, "compatibility": true,
			"keywords": true, "auto_inject": true, "author": true,
		}
		var unexpected []string
		for key := range fm {
			if !allowedFields[key] {
				unexpected = append(unexpected, key)
			}
		}
		if len(unexpected) > 0 {
			errors = append(errors, fmt.Sprintf(
				"Unexpected fields in frontmatter: %s", strings.Join(unexpected, ", ")))
		}
	}

	return errors
}

// findSkillMD returns the path to SKILL.md (preferred) or skill.md in dir.
// Returns empty string if neither exists.
func findSkillMD(dir string) string {
	for _, name := range []string{"SKILL.md", "skill.md"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}

// parseFrontmatterRaw extracts and parses the raw YAML frontmatter from a
// SKILL.md file. Returns nil map if the frontmatter is not a YAML mapping.
func parseFrontmatterRaw(path string) (map[string]any, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	text := string(content)
	fmText, err := extractFrontmatter(text)
	if err != nil {
		return nil, err
	}

	var result map[string]any
	if err := yaml.Unmarshal([]byte(fmText), &result); err != nil {
		return nil, fmt.Errorf("invalid YAML in frontmatter: %w", err)
	}
	return result, nil
}

// extractFrontmatter extracts the YAML frontmatter text between "---" markers.
func extractFrontmatter(text string) (string, error) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || !strings.HasPrefix(strings.TrimSpace(lines[0]), "---") {
		return "", fmt.Errorf("missing frontmatter delimiter (---)")
	}

	var fmLines []string
	foundClose := false
	for i := 1; i < len(lines); i++ {
		if strings.HasPrefix(strings.TrimSpace(lines[i]), "---") {
			foundClose = true
			break
		}
		fmLines = append(fmLines, lines[i])
	}
	if !foundClose {
		return "", fmt.Errorf("unclosed frontmatter (missing closing ---)")
	}
	return strings.Join(fmLines, "\n"), nil
}

// getStringField safely extracts a string value from a map.
func getStringField(m map[string]any, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

// validateName validates a skill name against spec rules.
// The name should already be NFKC normalized before calling this function.
//
// Rules:
//   - Must not be empty
//   - Max 64 characters
//   - All cased characters must be lowercase (Unicode-aware; CJK etc. are fine)
//   - Only letters, digits, and hyphens
//   - Cannot start or end with hyphen
//   - No consecutive hyphens
func validateName(name string) []string {
	var errors []string

	if name == "" {
		errors = append(errors, "Name must not be empty")
		return errors
	}

	if len(name) > 64 {
		errors = append(errors, fmt.Sprintf(
			"Name %q exceeds 64 character limit (%d chars)", name, len(name)))
	}

	// Must not start or end with hyphen
	if name[0] == '-' || name[len(name)-1] == '-' {
		errors = append(errors, "Name cannot start or end with a hyphen")
	}

	prevHyphen := false
	hasInvalidChar := false

	for _, r := range name {
		isLetter := unicode.IsLetter(r)
		isDigit := unicode.IsDigit(r)
		isHyphen := r == '-'

		if isLetter {
			// Only cased characters need to be lowercase; CJK etc. are fine
			if unicode.IsUpper(r) {
				errors = append(errors, fmt.Sprintf(
					"Name must be all lowercase: %q (offending char: %q)", name, r))
				break
			}
			prevHyphen = false
		} else if isDigit {
			prevHyphen = false
		} else if isHyphen {
			if prevHyphen {
				errors = append(errors, "Name cannot contain consecutive hyphens")
				break
			}
			prevHyphen = true
		} else {
			if !hasInvalidChar {
				errors = append(errors, fmt.Sprintf(
					"Name contains invalid character %q", r))
				hasInvalidChar = true
			}
			prevHyphen = false
		}
	}

	return errors
}