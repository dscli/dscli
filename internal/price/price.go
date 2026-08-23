package price

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Price holds per-million-token prices in yuan for one model.
type Price struct {
	PromptCacheHit  float64 `json:"prompt_cache_hit,omitzero"`
	PromptCacheMiss float64 `json:"prompt_cache_miss,omitzero"`
	Completion      float64 `json:"completion,omitzero"`
}

// peakPrice pairs the off-peak and peak rates announced on the pricing page
// for the 2026-08-17 pricing change.
type peakPrice struct {
	OffPeak Price `json:"off_peak"`
	Peak    Price `json:"peak"`
}

// priceCache is the parsed pricing page content, persisted to
// ~/.dscli/price.json so prices are fetched at most once a day.
type priceCache struct {
	FetchedAt time.Time            `json:"fetched_at"`
	Current   map[string]Price     `json:"current"`
	New       map[string]peakPrice `json:"new"`
}

// beijing is China Standard Time (UTC+8, no DST since 1991). The pricing
// page quotes peak hours and the new-price effective time in Beijing time.
var beijing = time.FixedZone("CST", 8*3600)

// newPriceEffective is when the peak/off-peak pricing takes effect,
// announced on the pricing page: 2026-08-17 00:00 Beijing time.
var newPriceEffective = time.Date(2026, 8, 17, 0, 0, 0, 0, beijing)

const (
	pricingURL    = "https://api-docs.deepseek.com/zh-cn/quick_start/pricing"
	cacheTTL      = 24 * time.Hour // fetch the pricing page at most once a day
	fetchRetryGap = time.Hour      // failed fetches retry no more often than hourly
)

var (
	theCache   *priceCache
	theCacheMu sync.Mutex
	lastFetch  time.Time // last fetch attempt, for failure backoff

	// cachePath is the on-disk cache location; overridable in tests.
	cachePath = defaultCachePath()
	// fetchPage fetches and parses the pricing page; overridable in tests.
	fetchPage = fetchPricingPage
)

// GetPrice returns the effective prices for the current time: the listed
// prices before 2026-08-17, and the new peak/off-peak prices afterwards,
// choosing peak prices during Beijing peak hours (9:00-12:00, 14:00-18:00).
// The pricing page is fetched at most once a day and cached on disk, so
// repeated calls do not hit the network. When the page cannot be reached,
// the last cached or built-in snapshot prices are used.
func GetPrice() map[string]Price {
	return getPrice(time.Now())
}

func getPrice(now time.Time) map[string]Price {
	c := loadCache(now)
	if c == nil {
		return nil
	}
	return c.resolve(now)
}

// loadCache returns the in-memory or on-disk cache, refreshing it from the
// pricing page once it is older than cacheTTL. A failed refresh keeps the
// stale cache; with no cache at all it falls back to the built-in snapshot.
func loadCache(now time.Time) *priceCache {
	theCacheMu.Lock()
	defer theCacheMu.Unlock()

	if theCache == nil {
		theCache = readCacheFile()
	}
	if theCache != nil && now.Sub(theCache.FetchedAt) < cacheTTL {
		return theCache
	}
	refresh(now)
	if theCache == nil {
		theCache = builtinCache()
	}
	return theCache
}

// refresh fetches the pricing page and replaces the cache on success. The
// lastFetch backoff prevents repeated network attempts inside a chat loop
// when the page is unreachable.
func refresh(now time.Time) {
	if now.Sub(lastFetch) < fetchRetryGap {
		return
	}
	lastFetch = now
	c, err := fetchPage()
	if err != nil {
		return
	}
	c.FetchedAt = now
	theCache = c
	saveCacheFile(c)
}

// resolve returns the effective prices at time t for every model with a
// known price in the active regime (listed before the 2026-08-17 change,
// peak/off-peak afterwards).
func (c *priceCache) resolve(t time.Time) map[string]Price {
	models := make(map[string]struct{}, len(c.Current)+len(c.New))
	for m := range c.Current {
		models[m] = struct{}{}
	}
	for m := range c.New {
		models[m] = struct{}{}
	}
	out := make(map[string]Price, len(models))
	for m := range models {
		if p, ok := c.priceFor(m, t); ok {
			out[m] = p
		}
	}
	return out
}

// priceFor returns the price of model at time t, or false when the model
// has no price in the regime active at t.
func (c *priceCache) priceFor(model string, t time.Time) (Price, bool) {
	t = t.In(beijing)
	if t.Before(newPriceEffective) {
		p, ok := c.Current[model]
		return p, ok
	}
	np, ok := c.New[model]
	if !ok {
		return Price{}, false
	}
	if inPeakHours(t) {
		return np.Peak, true
	}
	return np.OffPeak, true
}

// inPeakHours reports whether t (already in Beijing time) falls in the
// peak periods. Per the pricing page footnote (2026-08-22 snapshot) the
// peak windows are Monday-Friday 9:00-12:00 and 14:00-18:00; weekends are
// always off peak.
func inPeakHours(t time.Time) bool {
	if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
		return false
	}
	h := t.Hour()
	return (h >= 9 && h < 12) || (h >= 14 && h < 18)
}

// builtinCache returns the prices parsed from the pricing page snapshot
// taken 2026-08 (the page's main table plus the announced new rates). It is
// the last-resort fallback when the page is unreachable and no cache exists.
func builtinCache() *priceCache {
	// deepseek-v4-flash-vision-exp (added 2026-08-17) prices its tokens
	// identically to deepseek-v4-flash; only images are billed separately
	// via their token conversion.
	flashNew := peakPrice{
		OffPeak: Price{PromptCacheHit: 0.05, PromptCacheMiss: 1.5, Completion: 4.5},
		Peak:    Price{PromptCacheHit: 0.10, PromptCacheMiss: 3.0, Completion: 9.0},
	}
	proNew := peakPrice{
		OffPeak: Price{PromptCacheHit: 0.15, PromptCacheMiss: 4.5, Completion: 13.5},
		Peak:    Price{PromptCacheHit: 0.30, PromptCacheMiss: 9.0, Completion: 27.0},
	}
	return &priceCache{
		Current: map[string]Price{
			"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
			"deepseek-v4-pro":   {PromptCacheHit: 0.025, PromptCacheMiss: 3.0, Completion: 6.0},
		},
		New: map[string]peakPrice{
			"deepseek-v4-flash":            flashNew,
			"deepseek-v4-pro":              proNew,
			"deepseek-v4-flash-vision-exp": flashNew,
		},
	}
}

// fetchPricingPage downloads the pricing page and parses the current price
// table and the announced new-rate table. A page whose layout changed
// entirely is reported as an error so the last good cache is kept.
func fetchPricingPage() (*priceCache, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(pricingURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	html := normalizePricingHTML(string(b))
	c := builtinCache()
	currentOK := false
	if m := parsePrice(html); m != nil {
		c.Current = m
		currentOK = true
	}
	newOK := false
	if m := parseNewPrices(html); m != nil {
		c.New = m
		newOK = true
	}
	if !currentOK && !newOK {
		return nil, fmt.Errorf("unrecognized pricing page layout")
	}
	return c, nil
}

// normalizePricingHTML strips cell attributes (except rowspan, which the
// footnote parser anchors on) and whitespace between tags, so the exact
// string parsers below tolerate cosmetic HTML changes: style attributes on
// <td> cells or pretty-printed newlines must not silently break parsing.
func normalizePricingHTML(html string) string {
	html = tdTagRe.ReplaceAllString(html, "<td$1>")
	return tagGapRe.ReplaceAllString(html, "><")
}

var (
	// tdTagRe matches a <td ...> opening tag, keeping only a rowspan
	// attribute (e.g. <td rowspan="2" style="..."> -> <td rowspan="2">).
	tdTagRe = regexp.MustCompile(`<td((?:\s+rowspan="[0-9]+")?)[^>]*>`)
	// tagGapRe collapses whitespace between tags: "</td>\n<tr>" -> "</td><tr>".
	tagGapRe = regexp.MustCompile(`>\s+<`)
)

// parsePrice extracts the listed prices from the main price table.
func parsePrice(html string) (price map[string]Price) {
	flash := Price{}
	pro := Price{}
	_, after, found := strings.Cut(html, ">价格</td><td>")
	if !found {
		// 2026-07 页面改版后，"价格"单元格带脚注标注：
		//   <td rowspan="3">价格<sup>(1)</sup></td><td>百万tokens输入（缓存命中）</td><td>0.02元</td>...
		// 跳过 <sup>…</sup> 后与上面的路径汇合，后续解析逻辑不变。
		_, after, found = strings.Cut(html, ">价格<sup>")
		if !found {
			return
		}
		_, after, found = strings.Cut(after, "</sup></td><td>")
		if !found {
			return
		}
	}
	_, after, found = strings.Cut(after, "</td><td>")
	if !found {
		return
	}

	before, after, found := strings.Cut(after, "元</td><td>")
	if !found {
		return
	}
	f, err := strconv.ParseFloat(before, 64)
	if err != nil {
		return
	}

	flash.PromptCacheHit = f
	before, after, found = strings.Cut(after, "元</td>")
	if !found {
		return
	}

	f, err = strconv.ParseFloat(before, 64)
	if err != nil {
		return
	}
	pro.PromptCacheHit = f
	_, after, found = strings.Cut(after, "</td><td>")
	if !found {
		return
	}
	before, after, found = strings.Cut(after, "元</td><td>")
	if !found {
		return
	}

	f, err = strconv.ParseFloat(before, 64)
	if err != nil {
		return
	}
	flash.PromptCacheMiss = f

	before, after, found = strings.Cut(after, "元</td>")
	if !found {
		return
	}
	f, err = strconv.ParseFloat(before, 64)
	if err != nil {
		return
	}
	pro.PromptCacheMiss = f
	_, after, found = strings.Cut(after, "</td><td>")
	if !found {
		return
	}
	before, after, found = strings.Cut(after, "元</td><td>")
	if !found {
		return
	}
	f, err = strconv.ParseFloat(before, 64)
	if err != nil {
		return
	}
	flash.Completion = f
	before, after, found = strings.Cut(after, "元</td>")
	if !found {
		return
	}
	f, err = strconv.ParseFloat(before, 64)
	if err != nil {
		return
	}
	pro.Completion = f
	price = map[string]Price{
		"deepseek-v4-flash": flash,
		"deepseek-v4-pro":   pro,
	}
	return
}

// parseNewPrices extracts the peak/off-peak prices for each model. Two page
// layouts have existed: before 2026-08-17 the rates lived in a footnote (1)
// table introduced by "具体如下：", while the post-change page embeds them
// in the main price table under a "价格(1)" cell. Both are supported so a
// page rollback cannot break parsing. The footnote layout is tried first —
// when both appear (e.g. during a transition period) the footnote wins.
//
// Since the 2026-08-22 pricing-page update, footnote (1) is plain prose
// ("空闲时段价格为高峰时段价格的一半...") with no table; parseNewPricesFootnote
// simply returns nil for it and the main table is used.
func parseNewPrices(html string) map[string]peakPrice {
	if m := parseNewPricesFootnote(html); m != nil {
		return m
	}
	return parseNewPricesMainTable(html)
}

// parseNewPricesFootnote parses the pre-2026-08-17 layout, where the new
// peak/off-peak rates were announced in footnote (1) of the pricing page:
//
//	<tr><td rowspan="2">deepseek-v4-flash</td><td>空闲时段</td><td>0.05元</td>...
//	<tr><td>高峰时段</td><td>0.10元</td>...
func parseNewPricesFootnote(html string) map[string]peakPrice {
	_, rest, found := strings.Cut(html, "具体如下：")
	if !found {
		return nil
	}
	prices := map[string]peakPrice{}
	any := false
	for {
		_, rest, found = strings.Cut(rest, `<td rowspan="2">`)
		if !found {
			break
		}
		model, rest, found := strings.Cut(rest, "</td><td>")
		if !found {
			return nil
		}
		// Skip the period label cell ("空闲时段") and parse the three rates.
		_, rest, found = strings.Cut(rest, "</td><td>")
		if !found {
			return nil
		}
		off, rest, found := parseThreePrices(rest)
		if !found {
			return nil
		}
		_, rest, found = strings.Cut(rest, "<td>高峰时段</td><td>")
		if !found {
			return nil
		}
		peak, rest, found := parseThreePrices(rest)
		if !found {
			return nil
		}
		prices[model] = peakPrice{OffPeak: off, Peak: peak}
		any = true
	}
	if !any {
		return nil
	}
	return prices
}

// parseNewPricesMainTable parses the post-2026-08-17 layout, where the main
// price table carries a single "价格" cell (footnote-marked) whose rows
// alternate 空闲时段 / 高峰时段 rates for every model. The number of model
// columns is variable - two initially, three since the
// deepseek-v4-flash-vision-exp addition (which prices like flash):
//
//	<tr><td rowspan="6">价格<sup>(1)(2)</sup></td><td rowspan="2">百万tokens输入（缓存命中）</td><td>空闲时段</td><td>0.05元</td><td>0.15元</td><td>0.05元</td></tr>
//	<tr><td>高峰时段</td><td>0.10元</td><td>0.30元</td><td>0.10元</td></tr>
//	<tr><td rowspan="2">百万tokens输入（缓存未命中）</td><td>空闲时段</td><td>1.5元</td><td>4.5元</td><td>1.5元</td></tr>
//	<tr><td>高峰时段</td><td>3.0元</td><td>9.0元</td><td>3.0元</td></tr>
//	<tr><td rowspan="2">百万tokens输出</td><td>空闲时段</td><td>4.5元</td><td>13.5元</td><td>4.5元</td></tr>
//	<tr><td>高峰时段</td><td>9.0元</td><td>27.0元</td><td>9.0元</td></tr>
func parseNewPricesMainTable(html string) map[string]peakPrice {
	_, rest, found := strings.Cut(html, ">价格<sup>")
	if !found {
		return nil
	}
	// Skip the footnote markers ((1) or (1)(2)) inside the sup element.
	_, rest, found = strings.Cut(rest, "</sup></td>")
	if !found {
		return nil
	}

	// Six rows - 缓存命中/未命中/输出 × 空闲/高峰 - each carrying every
	// model's rate in column order. The peak flag pins each row to the
	// expected 空闲/高峰 alternation so a reordered page fails loudly
	// instead of silently assigning rates to the wrong fields.
	var rows [6][]float64
	for i := 0; i < 6; i++ {
		peak, row, after, ok := parseRateRow(rest)
		if !ok || peak != (i%2 == 1) {
			return nil
		}
		rows[i] = row
		rest = after
	}
	n := len(rows[0])
	if n < 2 {
		return nil
	}
	for _, row := range rows[1:] {
		if len(row) != n {
			return nil
		}
	}

	models := parseModelNames(html)
	if models == nil {
		// Header missing (trimmed fragment): fall back to the built-in
		// model names, taking as many as there are price columns.
		if n > len(defaultModelNames) {
			return nil
		}
		models = defaultModelNames[:n]
	}
	if len(models) != n {
		return nil
	}
	seen := make(map[string]bool, n)
	for _, m := range models {
		if seen[m] {
			return nil
		}
		seen[m] = true
	}

	out := make(map[string]peakPrice, n)
	for j, m := range models {
		out[m] = peakPrice{
			OffPeak: Price{
				PromptCacheHit:  rows[0][j],
				PromptCacheMiss: rows[2][j],
				Completion:      rows[4][j],
			},
			Peak: Price{
				PromptCacheHit:  rows[1][j],
				PromptCacheMiss: rows[3][j],
				Completion:      rows[5][j],
			},
		}
	}
	return out
}

// defaultModelNames are the fallback model IDs used when the main price
// table header is missing from a page fragment; parseNewPricesMainTable
// takes as many leading names as there are price columns.
var defaultModelNames = []string{
	"deepseek-v4-flash",
	"deepseek-v4-pro",
	"deepseek-v4-flash-vision-exp",
}

// parseModelNames reads the model names from the main table header
// ("<td colspan="3">模型</td><td>deepseek-v4-flash</td><td>deepseek-v4-pro</td>
// <td>deepseek-v4-flash-vision-exp</td>"), returning nil when the header is
// missing or malformed (e.g. in trimmed page fragments). The number of
// models is variable - it grew from 2 to 3 in 2026-08. Parsing stops at the
// end of the header row: the price rows below use the same "</td><td>"
// separator and would otherwise leak into the name list.
func parseModelNames(html string) []string {
	_, after, found := strings.Cut(html, ">模型</td><td>")
	if !found {
		return nil
	}
	row, _, found := strings.Cut(after, "</tr>")
	if !found {
		row = after
	}
	var models []string
	for {
		m, rest, found := strings.Cut(row, "</td><td>")
		if !found {
			m, _, found = strings.Cut(row, "</td>")
			if !found {
				return nil
			}
			return append(models, m)
		}
		models = append(models, m)
		row = rest
	}
}

// parseRateRow reads one rate row of the form
// "<td>百万tokens输入（缓存命中）</td><td>空闲时段</td><td>0.05元</td><td>0.15元" or
// "<td>高峰时段</td><td>0.10元</td><td>0.30元" (continuation rows carry no
// metric cell) from the start of rest and returns whether it is a peak row,
// every model's rate in column order, and the remaining text. The period
// label is the first "时段</td><td>" in rest - metric names never contain
// "时段", so this cannot skip ahead to a later row. The row ends at the
// first "</td></tr>", so any number of model columns is accepted.
func parseRateRow(rest string) (peak bool, rates []float64, after string, ok bool) {
	before, after, found := strings.Cut(rest, "时段</td><td>")
	if !found {
		return false, nil, "", false
	}
	switch {
	case strings.HasSuffix(before, "<td>高峰"):
		peak = true
	case strings.HasSuffix(before, "<td>空闲"):
		// off-peak row
	default:
		return false, nil, "", false
	}
	for {
		before, after, found = strings.Cut(after, "元</td>")
		if !found {
			return false, nil, "", false
		}
		f, err := strconv.ParseFloat(before, 64)
		if err != nil {
			return false, nil, "", false
		}
		rates = append(rates, f)
		if strings.HasPrefix(after, "</tr>") {
			return peak, rates, after, true
		}
		after, found = strings.CutPrefix(after, "<td>")
		if !found {
			return false, nil, "", false
		}
	}
}

// parseThreePrices reads three prices of the form
// "0.05元</td><td>1.5元</td><td>4.5元" from the start of rest and returns
// them plus the remaining text.
func parseThreePrices(rest string) (Price, string, bool) {
	var p Price
	var err error
	before, after, found := strings.Cut(rest, "元</td><td>")
	if !found {
		return Price{}, "", false
	}
	if p.PromptCacheHit, err = strconv.ParseFloat(before, 64); err != nil {
		return Price{}, "", false
	}
	before, after, found = strings.Cut(after, "元</td><td>")
	if !found {
		return Price{}, "", false
	}
	if p.PromptCacheMiss, err = strconv.ParseFloat(before, 64); err != nil {
		return Price{}, "", false
	}
	before, after, found = strings.Cut(after, "元</td>")
	if !found {
		return Price{}, "", false
	}
	if p.Completion, err = strconv.ParseFloat(before, 64); err != nil {
		return Price{}, "", false
	}
	return p, after, true
}

func defaultCachePath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".dscli", "price.json")
}

func readCacheFile() *priceCache {
	if cachePath == "" {
		return nil
	}
	b, err := os.ReadFile(cachePath)
	if err != nil {
		return nil
	}
	var c priceCache
	if err := json.Unmarshal(b, &c); err != nil {
		return nil
	}
	if c.FetchedAt.IsZero() || c.Current == nil || c.New == nil {
		return nil
	}
	return &c
}

func saveCacheFile(c *priceCache) {
	if cachePath == "" {
		return
	}
	b, err := json.Marshal(c)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return
	}
	tmp := cachePath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return
	}
	_ = os.Rename(tmp, cachePath)
}
