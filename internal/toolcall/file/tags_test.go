package file

import (
	"testing"
)

func TestComputeLineTag(t *testing.T) {
	// Deterministic: same input always gives same tag
	tag1 := computeLineTag("int count = 10;")
	tag2 := computeLineTag("int count = 10;")
	if tag1 != tag2 {
		t.Errorf("same input should give same tag: %q vs %q", tag1, tag2)
	}

	// Different input should (usually) give different tag
	tag3 := computeLineTag("int count = 11;")
	if tag1 == tag3 {
		t.Logf("collision (expected to be rare): %q == %q", tag1, tag3)
	}

	// Empty line
	tag4 := computeLineTag("")
	if len(tag4) != 4 {
		t.Errorf("tag for empty line should be 4 chars, got %q", tag4)
	}

	// Non-ASCII content
	tag5 := computeLineTag("中文测试")
	if len(tag5) != 4 {
		t.Errorf("tag for non-ASCII should be 4 chars, got %q", tag5)
	}

	// All chars should be from tagCharset
	for _, c := range tag1 {
		if !isTagChar(byte(c)) {
			t.Errorf("tag contains invalid char %q in %q", c, tag1)
		}
	}
}

func isTagChar(c byte) bool {
	return (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_'
}

func TestParseLineTags(t *testing.T) {
	// Valid
	tags, err := parseLineTags("rA3_\nKq9z\nPX0b")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tags) != 3 {
		t.Fatalf("expected 3 tags, got %d", len(tags))
	}

	// Empty
	_, err = parseLineTags("")
	if err == nil {
		t.Error("expected error for empty input")
	}

	// Too short
	_, err = parseLineTags("abc")
	if err == nil {
		t.Error("expected error for 3-char tag")
	}

	// Too long
	_, err = parseLineTags("abcde")
	if err == nil {
		t.Error("expected error for 5-char tag")
	}

	// Invalid chars
	_, err = parseLineTags("ab+c")
	if err == nil {
		t.Error("expected error for tag with + char")
	}
}

func TestVerifyLineTags(t *testing.T) {
	lines := []string{
		"int count = 10;",
		"if (count > limit) {",
		"    count = limit;",
		"}",
	}

	// Compute correct tags
	tags := make([]string, len(lines))
	for i, line := range lines {
		tags[i] = computeLineTag(line)
	}

	// Should pass
	err := verifyLineTags(lines, 0, tags)
	if err != nil {
		t.Fatalf("verification should pass: %v", err)
	}

	// Should fail on wrong tag
	err = verifyLineTags(lines, 0, []string{"AAAA"})
	if err == nil {
		t.Error("verification should fail on wrong tag")
	}

	// Should fail on out-of-range line index
	err = verifyLineTags(lines, 10, tags)
	if err == nil {
		t.Error("verification should fail when line index out of range")
	}
}
