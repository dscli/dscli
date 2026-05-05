package tokenizer

import (
	"embed"
	"strings"
	"sync"
	"unicode"
)

//go:embed stopwords/*.txt
var stopwordsFS embed.FS

var (
	stopwordsOnce sync.Once
	stopwordsSet  map[string]bool
)

func initStopwords() {
	stopwordsOnce.Do(func() {
		stopwordsSet = make(map[string]bool, 3000)
		entries, err := stopwordsFS.ReadDir("stopwords")
		if err != nil {
			panic("tokenizer: failed to read stopwords dir: " + err.Error())
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".txt") {
				continue
			}
			data, err := stopwordsFS.ReadFile("stopwords/" + entry.Name())
			if err != nil {
				continue
			}
			for line := range strings.SplitSeq(string(data), "\n") {
				w := strings.TrimSpace(line)
				if w == "" || !containsCJK(w) {
					// Skip empty lines and entries without CJK characters
					// (pure ASCII, numbers, symbols — not Chinese stopwords).
					continue
				}
				stopwordsSet[w] = true
			}
		}
	})
}

// containsCJK reports whether s contains at least one CJK Unified Ideograph.
func containsCJK(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// IsStop reports whether word is a stopword — a high-frequency, low-meaning
// word that should be excluded from full-text search indexing and queries.
//
// It uses 3 popular Chinese stopword lists (embedded):
//   - cn_stopwords.txt  (Chinese function words, highest quality)
//   - hit_stopwords.txt (HIT Information Retrieval Lab)
//   - scu_stopwords.txt (Sichuan University, phrase-heavy)
//
// Only entries containing CJK characters are kept; pure ASCII/symbol entries
// are skipped to avoid filtering English content words.  This is why
// baidu_stopwords.txt was rejected: 39% of its entries are English words
// like "hello", "go", "the" — real content words that must not be filtered.
func IsStop(word string) bool {
	initStopwords()
	return stopwordsSet[word]
}
