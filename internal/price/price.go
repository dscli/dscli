package price

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
// peak periods 9:00-12:00 and 14:00-18:00.
func inPeakHours(t time.Time) bool {
	h := t.Hour()
	return (h >= 9 && h < 12) || (h >= 14 && h < 18)
}

// builtinCache returns the prices parsed from the pricing page snapshot
// taken 2026-08 (the page's main table plus the announced new rates). It is
// the last-resort fallback when the page is unreachable and no cache exists.
func builtinCache() *priceCache {
	return &priceCache{
		Current: map[string]Price{
			"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
			"deepseek-v4-pro":   {PromptCacheHit: 0.025, PromptCacheMiss: 3.0, Completion: 6.0},
		},
		New: map[string]peakPrice{
			"deepseek-v4-flash": {
				OffPeak: Price{PromptCacheHit: 0.05, PromptCacheMiss: 1.5, Completion: 4.5},
				Peak:    Price{PromptCacheHit: 0.10, PromptCacheMiss: 3.0, Completion: 9.0},
			},
			"deepseek-v4-pro": {
				OffPeak: Price{PromptCacheHit: 0.15, PromptCacheMiss: 4.5, Completion: 13.5},
				Peak:    Price{PromptCacheHit: 0.30, PromptCacheMiss: 9.0, Completion: 27.0},
			},
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

	c := builtinCache()
	currentOK := false
	if m := parsePrice(string(b)); m != nil {
		c.Current = m
		currentOK = true
	}
	newOK := false
	if m := parseNewPrices(string(b)); m != nil {
		c.New = m
		newOK = true
	}
	if !currentOK && !newOK {
		return nil, fmt.Errorf("unrecognized pricing page layout")
	}
	return c, nil
}

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

// parseNewPrices extracts the peak/off-peak table from footnote (1) of the
// pricing page, announced to take effect 2026-08-17:
//
//	<tr><td rowspan="2">deepseek-v4-flash</td><td>空闲时段</td><td>0.05元</td>...
//	<tr><td>高峰时段</td><td>0.10元</td>...
func parseNewPrices(html string) map[string]peakPrice {
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
