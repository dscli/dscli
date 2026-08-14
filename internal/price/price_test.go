package price

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestParsePrice(t *testing.T) {
	tcs := []struct {
		html string
		want map[string]Price
	}{
		{`<tr><td rowspan="3">价格</td><td>百万tokens输入（缓存命中）
</td><td>0.02元</td><td>0.025元</td>
</tr><tr><td>百万tokens输入（缓存未命中）
</td><td>1元</td><td>3元</td></tr><tr>
<td>百万tokens输出</td><td>2元</td><td>6元</td></tr>`, map[string]Price{
			"deepseek-v4-flash": {0.02, 1.0, 2.0},
			"deepseek-v4-pro":   {0.025, 3.0, 6.0},
		}},
		{`<table style="text-align: center;"><tr>
<td colspan="2" style="text-align: center;">模型</td><td>deepseek-v4-flash<sup>(1)</sup></td>
<td>deepseek-v4-pro</td></tr><tr><td colspan="2">BASE URL (OpenAI 格式)</td>
<td colspan="2"><a href="https://api.deepseek.com" target="_blank" rel="noopener noreferrer">https://api.deepseek.com</a></td></tr><tr>
<td colspan="2">BASE URL (Anthropic 格式)</td>
<td colspan="2"><a href="https://api.deepseek.com/anthropic" target="_blank" rel="noopener noreferrer">https://api.deepseek.com/anthropic</a></td>
</tr><tr><td colspan="2" style="text-align: center;">模型版本</td><td>DeepSeek-V4-Flash</td>
<td>DeepSeek-V4-Pro</td></tr><tr><td colspan="2">思考模式</td><td colspan="2">支持非思考与思考模式（默认）
<br>切换方式详见<a href="/zh-cn/guides/thinking_mode">思考模式</a></td></tr><tr><td colspan="2">上下文长度</td>
<td colspan="2">1M</td></tr><tr><td colspan="2">输出长度</td><td colspan="2">最大 384K</td></tr>
<tr><td rowspan="4">功能</td><td><a href="/zh-cn/guides/json_mode">Json Output</a></td><td>支持</td>
<td>支持</td></tr><tr><td><a href="/zh-cn/guides/tool_calls">Tool Calls</a></td><td>支持</td><td>支持</td></tr>
<tr><td><a href="/zh-cn/guides/chat_prefix_completion">对话前缀续写（Beta）</a></td><td>支持</td><td>支持</td></tr>
<tr><td><a href="/zh-cn/guides/fim_completion">FIM 补全（Beta）</a></td><td>仅非思考模式支持</td><td>仅非思考模式支持</td></tr>
<tr><td rowspan="3">价格</td><td>百万tokens输入（缓存命中）</td><td>0.02元</td><td>0.025元</td></tr>
<tr><td>百万tokens输入（缓存未命中）</td><td>1元</td><td>3元</td></tr>
<tr><td>百万tokens输出</td><td>2元</td><td>6元</td></tr>
<tr><td colspan="2">并发限制<sup>(2)</sup></td><td>2500</td><td>500</td></tr></table>`, map[string]Price{
			"deepseek-v4-flash": {0.02, 1.0, 2.0},
			"deepseek-v4-pro":   {0.025, 3.0, 6.0},
		}},
		{`<tr><td rowspan="3">价格<sup>(2)</sup></td><td>百万tokens输入（缓存命中）</td><td>0.02元</td><td>0.025元</td></tr><tr><td>百万tokens输入（缓存未命中）</td><td>1元</td><td>3元</td></tr><tr><td>百万tokens输出</td><td>2元</td><td>6元</td></tr>`, map[string]Price{
			"deepseek-v4-flash": {0.02, 1.0, 2.0},
			"deepseek-v4-pro":   {0.025, 3.0, 6.0},
		}},
	}
	for i, tc := range tcs {
		name := fmt.Sprintf("%d", i)
		t.Run(name, func(t *testing.T) {
			want := parsePrice(tc.html)
			if !reflect.DeepEqual(want, tc.want) {
				t.Fatal(want)
			}
		})
	}
}

func TestParseNewPrices(t *testing.T) {
	// 脚注(1) 新价格表的真实 HTML 结构（2026-08 页面快照）。
	html := `(1) 我们将对 DeepSeek API 价格进行更新调整，采用峰谷定价，空闲时段价格为高峰时段价格的一半。高峰时段为北京时间 9:00 - 12:00、14:00 - 18:00（其余为空闲时段）。新价格将于北京时间 2026 年 8 月 17 日 00:00 开始生效，具体如下：
<div style="font-size: 14px;"><table style="text-align: center;"><tr><td colspan="2">模型</td><td>百万tokens输入（缓存命中）</td><td>百万tokens输入（缓存未命中）</td><td>百万tokens输出</td></tr><tr><td rowspan="2">deepseek-v4-flash</td><td>空闲时段</td><td>0.05元</td><td>1.5元</td><td>4.5元</td></tr><tr><td>高峰时段</td><td>0.10元</td><td>3.0元</td><td>9.0元</td></tr><tr><td rowspan="2">deepseek-v4-pro</td><td>空闲时段</td><td>0.15元</td><td>4.5元</td><td>13.5元</td></tr><tr><td>高峰时段</td><td>0.30元</td><td>9.0元</td><td>27.0元</td></tr></table></div>`
	want := map[string]peakPrice{
		"deepseek-v4-flash": {
			OffPeak: Price{PromptCacheHit: 0.05, PromptCacheMiss: 1.5, Completion: 4.5},
			Peak:    Price{PromptCacheHit: 0.10, PromptCacheMiss: 3.0, Completion: 9.0},
		},
		"deepseek-v4-pro": {
			OffPeak: Price{PromptCacheHit: 0.15, PromptCacheMiss: 4.5, Completion: 13.5},
			Peak:    Price{PromptCacheHit: 0.30, PromptCacheMiss: 9.0, Completion: 27.0},
		},
	}
	got := parseNewPrices(html)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("parseNewPrices = %v, want %v", got, want)
	}
}

func TestResolvePrices(t *testing.T) {
	c := builtinCache()
	flashOld := Price{PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0}
	proOld := Price{PromptCacheHit: 0.025, PromptCacheMiss: 3.0, Completion: 6.0}
	flashOff := Price{PromptCacheHit: 0.05, PromptCacheMiss: 1.5, Completion: 4.5}
	proOff := Price{PromptCacheHit: 0.15, PromptCacheMiss: 4.5, Completion: 13.5}
	flashPeak := Price{PromptCacheHit: 0.10, PromptCacheMiss: 3.0, Completion: 9.0}
	proPeak := Price{PromptCacheHit: 0.30, PromptCacheMiss: 9.0, Completion: 27.0}

	tests := []struct {
		name string
		now  time.Time
		want map[string]Price
	}{
		{"before effective date, peak hour keeps listed prices", time.Date(2026, 8, 16, 10, 0, 0, 0, beijing), map[string]Price{"deepseek-v4-flash": flashOld, "deepseek-v4-pro": proOld}},
		{"effective date midnight is off peak", time.Date(2026, 8, 17, 0, 0, 0, 0, beijing), map[string]Price{"deepseek-v4-flash": flashOff, "deepseek-v4-pro": proOff}},
		{"morning peak 09:00", time.Date(2026, 8, 17, 9, 0, 0, 0, beijing), map[string]Price{"deepseek-v4-flash": flashPeak, "deepseek-v4-pro": proPeak}},
		{"noon boundary 12:00 is off peak", time.Date(2026, 8, 17, 12, 0, 0, 0, beijing), map[string]Price{"deepseek-v4-flash": flashOff, "deepseek-v4-pro": proOff}},
		{"afternoon peak starts 14:00", time.Date(2026, 8, 17, 14, 0, 0, 0, beijing), map[string]Price{"deepseek-v4-flash": flashPeak, "deepseek-v4-pro": proPeak}},
		{"afternoon peak ends 18:00", time.Date(2026, 8, 17, 18, 0, 0, 0, beijing), map[string]Price{"deepseek-v4-flash": flashOff, "deepseek-v4-pro": proOff}},
		{"utc input converts to beijing", time.Date(2026, 8, 17, 1, 0, 0, 0, time.UTC), map[string]Price{"deepseek-v4-flash": flashPeak, "deepseek-v4-pro": proPeak}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := c.resolve(tt.now)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("resolve(%v) = %v, want %v", tt.now, got, tt.want)
			}
		})
	}
}

func TestResolvePriceForUnknownModel(t *testing.T) {
	c := builtinCache()
	// 新价格表中没有的模型，生效后不应返回价格。
	if p, ok := c.priceFor("deepseek-chat", time.Date(2026, 8, 17, 10, 0, 0, 0, beijing)); ok {
		t.Fatalf("expected no price, got %v", p)
	}
}

func resetPriceState() {
	theCacheMu.Lock()
	theCache = nil
	lastFetch = time.Time{}
	theCacheMu.Unlock()
}

func TestGetPriceCaching(t *testing.T) {
	origPath, origFetch := cachePath, fetchPage
	cachePath = filepath.Join(t.TempDir(), "price.json")
	t.Cleanup(func() { cachePath, fetchPage = origPath, origFetch })
	resetPriceState()

	fetches := 0
	fetchPage = func() (*priceCache, error) {
		fetches++
		return builtinCache(), nil
	}

	// now = 2026-08-16 22:00 北京时间：生效日前，返回现价表价格。
	// +25h = 8-17 23:00、+26h = 8-18 00:00、+49h = 8-18 23:00 均为空闲时段，
	// 新价格生效后统一返回谷价 0.05，便于断言。
	now := time.Date(2026, 8, 16, 22, 0, 0, 0, beijing)
	got := getPrice(now)
	if got["deepseek-v4-flash"].PromptCacheHit != 0.02 {
		t.Fatalf("unexpected prices: %v", got)
	}
	if fetches != 1 {
		t.Fatalf("expected 1 fetch on first call, got %d", fetches)
	}

	// 内存缓存命中：不重复抓取。
	getPrice(now.Add(time.Hour))
	if fetches != 1 {
		t.Fatalf("expected no refetch within TTL, got %d", fetches)
	}

	// 超过 TTL 后重新抓取。
	getPrice(now.Add(25 * time.Hour))
	if fetches != 2 {
		t.Fatalf("expected refetch after TTL, got %d", fetches)
	}

	// 缓存仍新鲜（+26h 距上次抓取仅 1h）：不触发抓取，直接服务缓存。
	got = getPrice(now.Add(26 * time.Hour))
	if got["deepseek-v4-flash"].PromptCacheHit != 0.05 {
		t.Fatalf("unexpected new off-peak prices: %v", got)
	}
	if fetches != 2 {
		t.Fatalf("expected no fetch while fresh, got %d", fetches)
	}

	// 超过 TTL 后抓取失败：保留旧缓存，且 1 小时内不重试。
	fetchPage = func() (*priceCache, error) {
		fetches++
		return nil, errors.New("network down")
	}
	got = getPrice(now.Add(49 * time.Hour))
	if got["deepseek-v4-flash"].PromptCacheHit != 0.05 {
		t.Fatalf("stale cache not served after failed fetch: %v", got)
	}
	if fetches != 3 {
		t.Fatalf("expected 3rd fetch attempt, got %d", fetches)
	}
	getPrice(now.Add(49*time.Hour + 30*time.Minute))
	if fetches != 3 {
		t.Fatalf("expected no retry within backoff, got %d", fetches)
	}

	// 磁盘缓存：清空内存状态后（模拟新进程）从磁盘恢复，不重新抓取。
	resetPriceState()
	got = getPrice(now.Add(26 * time.Hour))
	if fetches != 3 {
		t.Fatalf("expected disk cache hit without fetch, got %d", fetches)
	}
	if got["deepseek-v4-flash"].PromptCacheHit != 0.05 {
		t.Fatalf("disk cache not restored: %v", got)
	}
}

func TestGetPriceFallbackBuiltin(t *testing.T) {
	origPath, origFetch := cachePath, fetchPage
	cachePath = filepath.Join(t.TempDir(), "price.json") // 空目录，无磁盘缓存
	fetchPage = func() (*priceCache, error) {
		return nil, errors.New("network down")
	}
	t.Cleanup(func() { cachePath, fetchPage = origPath, origFetch })
	resetPriceState()

	got := getPrice(time.Now())
	if len(got) != 2 || got["deepseek-v4-flash"].PromptCacheHit != 0.02 {
		t.Fatalf("builtin snapshot not used: %v", got)
	}
}
