package price

import (
	"sync"
	"testing"
	"time"
)

// setTestPrices installs a fresh in-memory cache so GetCost never touches
// the network or the on-disk cache. The same prices are set for the current
// and the new peak/off-peak regime so the expected cost holds regardless of
// when the test runs.
func setTestPrices(current map[string]Price) {
	theCacheMu.Lock()
	newPrices := make(map[string]peakPrice, len(current))
	for m, p := range current {
		newPrices[m] = peakPrice{OffPeak: p, Peak: p}
	}
	theCache = &priceCache{
		FetchedAt: time.Now(),
		Current:   current,
		New:       newPrices,
	}
	theCacheMu.Unlock()
}

func TestGetCostZeroWhenNoPrices(t *testing.T) {
	// 没有价格数据时，任何模型都返回 0
	setTestPrices(nil)
	theUsage = Usage{
		PromptCacheHitTokens:  100,
		PromptCacheMissTokens: 200,
		CompletionTokens:      50,
	}
	cost := GetCost("unknown-model")
	if cost != 0 {
		t.Fatalf("expected 0 for unknown model, got %f", cost)
	}
}

func TestGetCostWithPrices(t *testing.T) {
	setTestPrices(map[string]Price{
		"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
	})
	theUsage = Usage{
		PromptCacheHitTokens:  1_000_000, // 1M tokens → 0.02 元
		PromptCacheMissTokens: 500_000,   // 0.5M tokens → 0.5 元
		CompletionTokens:      200_000,   // 0.2M tokens → 0.4 元
	}
	cost := GetCost("deepseek-v4-flash")
	expected := 0.02 + 0.5 + 0.4 // = 0.92
	if cost != expected {
		t.Fatalf("expected %f, got %f", expected, cost)
	}
}

func TestGetCostZeroUsage(t *testing.T) {
	setTestPrices(map[string]Price{
		"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
	})
	theUsage = Usage{}
	cost := GetCost("deepseek-v4-flash")
	if cost != 0 {
		t.Fatalf("expected 0 for zero usage, got %f", cost)
	}
}

func TestGetCostConcurrentSafe(t *testing.T) {
	setTestPrices(map[string]Price{
		"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
	})
	theUsage = Usage{
		PromptCacheHitTokens:  100,
		PromptCacheMissTokens: 200,
		CompletionTokens:      50,
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = GetCost("deepseek-v4-flash")
		}()
	}
	wg.Wait()
}
