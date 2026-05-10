package skills

import (
	"os"
	"path/filepath"
	"testing"
)

// =============================================================================
// ValidateSkillDir tests — aligned with skills-ref test_validator.py
// =============================================================================

func TestValidateValidSkill(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: A test skill\n---\n# My Skill\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestValidateNonexistentPath(t *testing.T) {
	errors := ValidateSkillDir(filepath.Join(t.TempDir(), "nonexistent"))
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
	if !contains(errors[0], "does not exist") {
		t.Errorf("error should mention 'does not exist': %s", errors[0])
	}
}

func TestValidateNotADirectory(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "file.txt")
	mustWrite(t, filePath, "test")

	errors := ValidateSkillDir(filePath)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
	if !contains(errors[0], "Not a directory") {
		t.Errorf("error should mention 'Not a directory': %s", errors[0])
	}
}

func TestValidateMissingSkillMD(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errors), errors)
	}
	if !contains(errors[0], "Missing required file: SKILL.md") {
		t.Errorf("error mismatch: %s", errors[0])
	}
}

func TestValidateNameUppercase(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "MySkill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: MySkill\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "lowercase") {
		t.Errorf("expected 'lowercase' error, got: %v", errors)
	}
}

func TestValidateNameTooLong(t *testing.T) {
	longName := "a" + string(make([]byte, 69))[1:] // 70 chars, exceeds 64 limit
	longName = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 69 a's
	dir := t.TempDir()
	skillDir := filepath.Join(dir, longName)
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: "+longName+"\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "exceeds") || !anyContains(errors, "character limit") {
		t.Errorf("expected length error, got: %v", errors)
	}
}

func TestValidateNameLeadingHyphen(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "-my-skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: -my-skill\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "cannot start or end with a hyphen") {
		t.Errorf("expected hyphen start/end error, got: %v", errors)
	}
}

func TestValidateNameConsecutiveHyphens(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my--skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my--skill\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "consecutive hyphens") {
		t.Errorf("expected consecutive hyphens error, got: %v", errors)
	}
}

func TestValidateNameInvalidCharacters(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my_skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my_skill\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "invalid character") {
		t.Errorf("expected invalid character error, got: %v", errors)
	}
}

func TestValidateNameDirectoryMismatch(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "wrong-name")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: correct-name\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "must match skill name") {
		t.Errorf("expected directory mismatch error, got: %v", errors)
	}
}

func TestValidateUnexpectedFields(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: A test skill\nunknown_field: should not be here\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "Unexpected fields") {
		t.Errorf("expected unexpected fields error, got: %v", errors)
	}
}

func TestValidateWithAllFields(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: A test skill\nlicense: MIT\nmetadata:\n  author: Test\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestValidateAllowedToolsAccepted(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: A test skill\nallowed-tools: Bash(jq:*) Bash(git:*)\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors for allowed-tools, got: %v", errors)
	}
}

func TestValidateChineseName(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "技能")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: 技能\ndescription: A skill with Chinese name\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors for Chinese name, got: %v", errors)
	}
}

func TestValidateRussianNameWithHyphens(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "мой-навык")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: мой-навык\ndescription: A skill with Russian name\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors for Russian name with hyphens, got: %v", errors)
	}
}

func TestValidateRussianLowercaseValid(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "навык")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: навык\ndescription: A skill with Russian lowercase name\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors for Russian lowercase, got: %v", errors)
	}
}

func TestValidateRussianUppercaseRejected(t *testing.T) {
	dir := t.TempDir()
	// IMPORTANT: use lowercase dir name since filepath.Join on some FS normalizes
	// For the uppercase test, the dir name needs to be uppercase too
	skillDir := filepath.Join(dir, "НАВЫК")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: НАВЫК\ndescription: A skill with Russian uppercase name\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "lowercase") {
		t.Errorf("expected lowercase error for Russian uppercase, got: %v", errors)
	}
}

func TestValidateDescriptionTooLong(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	longDesc := make([]byte, 1100)
	for i := range longDesc {
		longDesc[i] = 'x'
	}
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: "+string(longDesc)+"\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "exceeds") || !anyContains(errors, "1024") {
		t.Errorf("expected description length error, got: %v", errors)
	}
}

func TestValidateValidCompatibility(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: A test skill\ncompatibility: Requires Python 3.11+\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("expected no errors, got: %v", errors)
	}
}

func TestValidateCompatibilityTooLong(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "my-skill")
	mustMkdir(t, skillDir)
	longCompat := make([]byte, 550)
	for i := range longCompat {
		longCompat[i] = 'x'
	}
	writeSKILLMD(t, skillDir, "---\nname: my-skill\ndescription: A test skill\ncompatibility: "+string(longCompat)+"\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if !anyContains(errors, "exceeds") || !anyContains(errors, "500") {
		t.Errorf("expected compatibility length error, got: %v", errors)
	}
}

func TestValidateNFKCNormalization(t *testing.T) {
	// Decomposed form: 'cafe' + combining acute accent (U+0301)
	decomposedName := "cafe\u0301"
	composedName := "café" // precomposed

	dir := t.TempDir()
	skillDir := filepath.Join(dir, composedName)
	mustMkdir(t, skillDir)
	writeSKILLMD(t, skillDir, "---\nname: "+decomposedName+"\ndescription: A test skill\n---\nBody\n")

	errors := ValidateSkillDir(skillDir)
	if len(errors) != 0 {
		t.Errorf("NFKC normalization failed, expected no errors, got: %v", errors)
	}
}

// =============================================================================
// validateName unit tests
// =============================================================================

func TestValidateNameEmpty(t *testing.T) {
	errs := validateName("")
	if len(errs) == 0 {
		t.Error("expected error for empty name")
	}
}

func TestValidateNameASCIIValid(t *testing.T) {
	for _, name := range []string{"my-skill", "go-fix", "use-modern-go", "a", "test123", "123test"} {
		t.Run(name, func(t *testing.T) {
			errs := validateName(name)
			if len(errs) != 0 {
				t.Errorf("name %q should be valid, got: %v", name, errs)
			}
		})
	}
}

func TestValidateNameASCIIInvalid(t *testing.T) {
	invalid := map[string]string{
		"-leading":       "cannot start",
		"trailing-":      "cannot start or end",
		"double--hyphen": "consecutive",
		"_underscore":    "invalid character",
		"UPPERCASE":      "lowercase",
		"CamelCase":      "lowercase",
		"space name":     "invalid character",
		"special!":       "invalid character",
	}
	for name, want := range invalid {
		t.Run(name, func(t *testing.T) {
			errs := validateName(name)
			if len(errs) == 0 {
				t.Errorf("name %q should be invalid, got no errors", name)
			}
			if !anyContains(errs, want) {
				t.Errorf("name %q: expected error containing %q, got: %v", name, want, errs)
			}
		})
	}
}

// =============================================================================
// extractFrontmatter unit tests
// =============================================================================

func TestExtractFrontmatterValid(t *testing.T) {
	text := "---\nname: test\ndescription: desc\n---\nBody content"
	fm, err := extractFrontmatter(text)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fm != "name: test\ndescription: desc" {
		t.Errorf("got %q, want %q", fm, "name: test\ndescription: desc")
	}
}

func TestExtractFrontmatterMissingOpen(t *testing.T) {
	_, err := extractFrontmatter("name: test\n---\nBody")
	if err == nil {
		t.Error("expected error for missing opening ---")
	}
}

func TestExtractFrontmatterMissingClose(t *testing.T) {
	_, err := extractFrontmatter("---\nname: test\nBody")
	if err == nil {
		t.Error("expected error for missing closing ---")
	}
}

// =============================================================================
// findSkillMD tests
// =============================================================================

func TestFindSkillMDUppercasePreferred(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "test")

	got := findSkillMD(dir)
	if got != filepath.Join(dir, "SKILL.md") {
		t.Errorf("expected SKILL.md, got %q", got)
	}
}

func TestFindSkillMDLowercaseFallback(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "skill.md"), "test")

	got := findSkillMD(dir)
	if got != filepath.Join(dir, "skill.md") {
		t.Errorf("expected skill.md, got %q", got)
	}
}

func TestFindSkillMDPreferUppercase(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "SKILL.md"), "upper")
	mustWrite(t, filepath.Join(dir, "skill.md"), "lower")

	got := findSkillMD(dir)
	if got != filepath.Join(dir, "SKILL.md") {
		t.Errorf("SKILL.md should be preferred, got %q", got)
	}
}

// =============================================================================
// Helpers
// =============================================================================

func mustMkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSKILLMD(t *testing.T, skillDir, content string) {
	t.Helper()
	mustWrite(t, filepath.Join(skillDir, "SKILL.md"), content)
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func anyContains(errs []string, substr string) bool {
	for _, e := range errs {
		if contains(e, substr) {
			return true
		}
	}
	return false
}