package price

import (
	"sync"
	"testing"
)

func TestGetCostZeroWhenNoPrices(t *testing.T) {
	// 没有价格数据时，任何模型都返回 0
	thePrice = nil
	thePriceOnce = sync.Once{}
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
	// 设置价格数据
	thePrice = map[string]Price{
		"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
	}
	thePriceOnce = sync.Once{}
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
	thePrice = map[string]Price{
		"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
	}
	thePriceOnce = sync.Once{}
	theUsage = Usage{}
	cost := GetCost("deepseek-v4-flash")
	if cost != 0 {
		t.Fatalf("expected 0 for zero usage, got %f", cost)
	}
}

func TestGetCostConcurrentSafe(t *testing.T) {
	thePrice = map[string]Price{
		"deepseek-v4-flash": {PromptCacheHit: 0.02, PromptCacheMiss: 1.0, Completion: 2.0},
	}
	thePriceOnce = sync.Once{}
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
