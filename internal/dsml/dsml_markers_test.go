package dsml

import (
	"os"
	"strings"
	"testing"
)

// DSML marker samples are built at runtime (bars via a rune, the tag opener
// via lt from dsml_strayclose_test.go) so this file stays transportable: a
// literal full-width bar sequence in the source would be mangled by the
// transport channel (see the header of dsml_strayclose_test.go).
var fwBar = string(rune(0xFF5C))

// badgeOpen returns the site's stored render of an OPEN tag: '<' + bars +
// 'DSML' + bars + name + '>'.
func badgeOpen(name string) string {
	return lt + fwBar + fwBar + "DSML" + fwBar + fwBar + name + gt
}

// badgeClose returns the site's stored render of a CLOSE tag.
func badgeClose(name string) string {
	return lt + "/" + fwBar + fwBar + "DSML" + fwBar + fwBar + name + gt
}

func TestMarkerRanges(t *testing.T) {
	tests := []struct {
		name string
		text string
		want int // number of marker ranges
	}{
		{
			name: "fullwidth badge close parameter",
			text: "text " + badgeClose("parameter"),
			want: 1,
		},
		{
			name: "fullwidth badge close invoke and calls",
			text: badgeClose("invoke") + nl + badgeClose("_calls"),
			want: 2,
		},
		{
			name: "ascii bar variant",
			text: "x " + lt + "/||DSML||parameter" + gt,
			want: 1,
		},
		{
			name: "plain closes",
			text: lt + "/invoke" + gt + " and " + lt + "/parameter" + gt + " and " + lt + "/tool_calls" + gt,
			want: 3,
		},
		{
			name: "plain opens",
			text: lt + `invoke name=` + q + `shell` + q + gt + nl + lt + `tool_calls` + gt,
			want: 2,
		},
		{
			name: "split DSML letters and whitespace tolerated",
			text: lt + "/" + fwBar + " d s m l " + fwBar + "parameter" + gt,
			want: 1,
		},
		{
			name: "bare bars in prose never match",
			text: "a | b || c " + fwBar + " d " + fwBar + fwBar + " e",
			want: 0,
		},
		{
			name: "unrelated tag with dsml prefix is not a marker",
			text: "cat <dsml_config and a <d s m l b token",
			want: 0,
		},
		{
			name: "no markup at all",
			text: "plain prose, no angle brackets here",
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarkerRanges(tt.text)
			if len(got) != tt.want {
				t.Fatalf("MarkerRanges() returned %d ranges, want %d: %v", len(got), tt.want, got)
			}
			for _, r := range got {
				if r[0] < 0 || r[1] > len(tt.text) || r[0] >= r[1] {
					t.Errorf("range %v is not a valid slice of a %d-byte text", r, len(tt.text))
				}
			}
		})
	}
}

// TestMarkerRangesCoordinates pins the RAW-coordinate contract: the returned
// range must slice the matched bytes out of the original text, which is what
// lets callers (shellblock.Judge) subtract their own ranges.
func TestMarkerRangesCoordinates(t *testing.T) {
	const prefix = "done. "
	marker := badgeClose("parameter")
	text := prefix + marker + " trailing"
	ranges := MarkerRanges(text)
	if len(ranges) != 1 {
		t.Fatalf("ranges = %v, want exactly one", ranges)
	}
	if got := text[ranges[0][0]:ranges[0][1]]; got != marker {
		t.Errorf("sliced range = %q, want the marker bytes %q", got, marker)
	}
}

// TestMarkerRangesRealSamples feeds the byte-exact real captures: every one
// carries badge-rendered markers, so the predicate must find at least one.
func TestMarkerRangesRealSamples(t *testing.T) {
	for _, f := range []string{
		"testdata/case1_edit_argument.txt",
		"testdata/case2_write_file.txt",
		"testdata/real_sample2.txt",
		"testdata/case6_bare_short_close.txt",
	} {
		t.Run(f, func(t *testing.T) {
			b, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			text := string(b)
			ranges := MarkerRanges(text)
			if len(ranges) == 0 {
				t.Fatalf("no markers found in the real sample %s", f)
			}
			// Spot-check that at least one range slices out a tag-shaped
			// fragment (opener + name + closer), not arbitrary prose.
			found := false
			for _, r := range ranges {
				seg := text[r[0]:r[1]]
				if strings.Contains(seg, "invoke") || strings.Contains(seg, "parameter") ||
					strings.Contains(seg, "tool_calls") || strings.Contains(seg, "_calls") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no range in %s sliced a tag-shaped fragment", f)
			}
		})
	}
}

// TestMarkerRangesMergedAndSorted pins the merge: the badge noise arm
// matches a close tag up to its name, and the plain-close arm matches the
// canonical close IMMEDIATELY after it - adjacent matches must collapse into
// ONE range, so callers can treat the result as disjoint markers.
func TestMarkerRangesMergedAndSorted(t *testing.T) {
	// Arm 1 matches the badge close through "parameter"; arm 2 matches the
	// plain "</parameter>" glued to it. The trailing "</invoke>" stays its
	// own range. The EXACT expected ranges document the adjacency-based
	// merge contract: pair occupies [0, len(pair)) and the plain close of
	// invoke starts after the newline. The fixture relies on nl being a
	// non-empty separator - an empty nl would make the two arms touch and
	// collapse into one range.
	pair := lt + "/" + fwBar + fwBar + "DSML" + fwBar + fwBar + "parameter" + lt + "/parameter" + gt
	text := pair + nl + lt + "/invoke" + gt
	want := [][2]int{{0, len(pair)}, {len(pair) + len(nl), len(text)}}
	ranges := MarkerRanges(text)
	if len(ranges) != len(want) {
		t.Fatalf("ranges = %v, want exactly %v", ranges, want)
	}
	for i := range want {
		if ranges[i] != want[i] {
			t.Errorf("range %d = %v, want %v", i, ranges[i], want[i])
		}
	}
	for i := 1; i < len(ranges); i++ {
		if ranges[i][0] < ranges[i-1][1] {
			t.Errorf("ranges %v overlap: %v", i-1, ranges)
		}
		if ranges[i][0] < ranges[i-1][0] {
			t.Errorf("ranges not sorted: %v", ranges)
		}
	}
}
