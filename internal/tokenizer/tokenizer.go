// Package tokenizer provides Chinese+English word segmentation for FTS5
// full-text search indexing and querying.
//
// It uses gse (github.com/go-ego/gse) search-engine mode (CutSearch) which
// produces both compound words and their sub-words for better recall.
// Stopwords (high-frequency, low-meaning words like 的/了/也) are filtered out.
package tokenizer

import (
	"strings"
	"sync"

	"github.com/go-ego/gse"
)

var (
	seg     gse.Segmenter
	segOnce sync.Once
)

// ensureInit loads the gse Chinese dictionary once.
func ensureInit() {
	segOnce.Do(func() {
		// Load embedded simplified + traditional Chinese dictionary.
		// gse default dictionary covers ~150k words.
		if err := seg.LoadDictEmbed("zh"); err != nil {
			// Fallback: try without embed (custom dict path env).
			if err2 := seg.LoadDict(); err2 != nil {
				panic("tokenizer: failed to load gse dictionary: " + err.Error() + "; " + err2.Error())
			}
		}
	})
}

// Tokenize returns space-joined Chinese+English words for FTS5 indexing/search.
//
// It uses gse search-engine mode (CutSearch) for Chinese word segmentation,
// which produces both compound words and their sub-words for better recall.
// Whitespace, empty tokens, and stopwords are filtered.
//
// Examples:
//
//	"Go单元测试"        → "go 单元 测试 单元测试"
//	"SQLite轻量级数据库"  → "sqlite 轻量 量级 轻量级 数据 据库 数据库"
func Tokenize(s string) string {
	ensureInit()
	words := seg.CutSearch(s, true)
	return joinTokens(filterTokens(words))
}

// SanitizeFTS prepares a user query for FTS5 MATCH.
//
// It tokenizes the query using gse search-engine mode (same as indexing),
// filters stopwords, then wraps each token in double-quotes.  This mirrors
// how content is indexed so both sides share the same tokenization.
//
//	"全文搜索"       → `"全文" "搜索"`
//	"Go单元测试"     → `"go" "单元" "测试" "单元测试"`
//	"fix auth bug"  → `"fix" "auth" "bug"`
func SanitizeFTS(query string) string {
	ensureInit()
	words := seg.CutSearch(query, true)
	tokens := filterTokens(words)
	if len(tokens) == 0 {
		return ""
	}
	var clean []string
	for _, w := range tokens {
		// Strip existing quotes, then wrap.
		w = strings.Trim(w, `"`)
		clean = append(clean, `"`+w+`"`)
	}
	return strings.Join(clean, " ")
}

// filterTokens trims whitespace and removes empty entries and stopwords.
func filterTokens(words []string) []string {
	var out []string
	for _, w := range words {
		w = strings.TrimSpace(w)
		if w == "" || IsStop(w) {
			continue
		}
		out = append(out, w)
	}
	return out
}

// joinTokens joins tokens with a single space.
func joinTokens(tokens []string) string {
	return strings.Join(tokens, " ")
}
