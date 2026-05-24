package file

import (
	"fmt"
	"hash/crc32"
	"strings"
)

// tagCharset is the 63-character alphabet used for line tags:
// uppercase, lowercase, digits, and underscore.
// With 4 chars: 63^4 ≈ 15.7M values ≈ 24 bits.
const tagCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_"

// tagBase is len(tagCharset) = 63.
const tagBase = uint64(63)

// tagSpace is tagBase^4 = 15752961.
const tagSpace = tagBase * tagBase * tagBase * tagBase

// computeLineTag returns a 4-character checksum tag for the given line content.
// Uses CRC32-IEEE of the line bytes, modulo 63^4, encoded as 4 base-63 chars.
func computeLineTag(line string) string {
	h := crc32.ChecksumIEEE([]byte(line))
	v := uint64(h) % tagSpace
	tag := make([]byte, 4)
	for i := 3; i >= 0; i-- {
		tag[i] = tagCharset[v%tagBase]
		v /= tagBase
	}
	return string(tag)
}

// parseLineTags parses a line_tags string of the form:
//
//	tag1
//	tag2
//	tag3
//
// Each tag must be exactly 4 characters from the tagCharset.
// Returns the slice of tags and the count.
func parseLineTags(lineTags string) ([]string, error) {
	raw := strings.TrimSpace(lineTags)
	if raw == "" {
		return nil, fmt.Errorf("line_tags is empty")
	}
	parts := strings.Split(raw, "\n")
	tags := make([]string, 0, len(parts))
	for i, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) != 4 {
			return nil, fmt.Errorf("line_tags[%d]: tag %q must be exactly 4 characters, got %d", i, p, len(p))
		}
		for _, c := range p {
			if !strings.ContainsRune(tagCharset, c) {
				return nil, fmt.Errorf("line_tags[%d]: tag %q contains invalid character %q", i, p, c)
			}
		}
		tags = append(tags, p)
	}
	if len(tags) == 0 {
		return nil, fmt.Errorf("line_tags contains no valid tags")
	}
	return tags, nil
}

// verifyTag checks whether the given tag matches the current line content.
// Returns the actual tag if mismatch.
func verifyTag(line string, expectedTag string) (ok bool, actualTag string) {
	actualTag = computeLineTag(line)
	return actualTag == expectedTag, actualTag
}

// VerifyLineTags verifies that a slice of tags matches the actual lines
// starting at startLine (0-based index into lines slice).
// Returns an error describing the first mismatch, or nil if all match.
func verifyLineTags(lines []string, startLine int, expectedTags []string) error {
	for i, expected := range expectedTags {
		lineIdx := startLine + i
		if lineIdx >= len(lines) {
			return fmt.Errorf(
				"tag verification: line %d does not exist (file has %d lines, tags expect line %d)",
				lineIdx+1, len(lines), lineIdx+1,
			)
		}
		ok, actual := verifyTag(lines[lineIdx], expected)
		if !ok {
			return fmt.Errorf(
				"tag verification failed at line %d: expected tag %q, but actual content produces tag %q.\n"+
					"Actual line content: %s",
				lineIdx+1, expected, actual, lines[lineIdx],
			)
		}
	}
	return nil
}
