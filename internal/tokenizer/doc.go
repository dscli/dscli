// Package tokenizer is the cornerstone of Chinese full-text search in dscli.
//
// Without it, Chinese queries against FTS5 would fail completely — FTS5's
// default tokenizer splits on whitespace, but Chinese has no spaces between
// words.  "全文搜索" becomes a single undividable token, making substring
// search ("全文" or "搜索") impossible.
//
// # Tokenization (Tokenize)
//
// Uses gse (github.com/go-ego/gse) in search-engine mode (CutSearch), which
// produces both compound words AND their sub-words:
//
//	"Go单元测试"  →  "go 单元 测试 单元测试"
//	"轻量级数据库" →  "轻量 量级 轻量级 数据 据库 数据库"
//
// This dual-output strategy ensures high recall: a search for "单元" matches
// "Go单元测试", and "轻量级数据库" matches even if the user types "轻量" alone.
//
// # Query Sanitization (SanitizeFTS)
//
// FTS5 MATCH queries must mirror the indexing tokenization.  SanitizeFTS
// tokenizes the user query with the same gse pipeline, filters stopwords,
// then wraps each token in double-quotes:
//
//	"全文搜索"  →  `"全文" "搜索"`
//	"fix auth"  →  `"fix" "auth"`
//
// # Stopword Filtering
//
// Three embedded stopword lists (cn_stopwords, hit_stopwords, scu_stopwords)
// filter high-frequency function words (的/了/也/吧/吗/呢/是/一个) that carry
// no semantic meaning.  A CJK-character gate (containsCJK) auto-rejects
// non-Chinese entries (hello, go, the, ———, 123), preventing English content
// words from being mistakenly filtered.
//
// The popular baidu_stopwords list was deliberately excluded: 39% of its
// 1,396 entries are English words (hello, go, the, unit, test…) — real
// content words that would destroy search recall if filtered.
//
// # Design
//
//   - Lazy init: gse dictionary (~150k words) and stopword maps are loaded
//     once on first use (sync.Once), not at package import.
//   - Zero config: dictionaries and stopwords are embedded via go:embed,
//     requiring no external files or environment setup.
//   - Thread-safe: all shared state is behind sync.Once.
package tokenizer
