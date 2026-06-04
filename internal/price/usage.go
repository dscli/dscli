package price

import "sync"

var theUsage Usage
var theUsageLock sync.Mutex

type Usage struct {
	CompletionTokens        int                      `json:"completion_tokens,omitzero"`
	PromptTokens            int                      `json:"prompt_tokens,omitzero"`
	PromptCacheHitTokens    int                      `json:"prompt_cache_hit_tokens,omitzero"`
	PromptCacheMissTokens   int                      `json:"prompt_cache_miss_tokens,omitzero"`
	TotalTokens             int                      `json:"total_tokens,omitzero"`
	CompletionTokensDetails *CompletionTokensDetails `json:"completion_tokens_details,omitzero"`
}

type CompletionTokensDetails struct {
	ReasoningTokens int `json:"reasoning_tokens,omitzero"`
}

func GetUsage() Usage {
	theUsageLock.Lock()
	defer theUsageLock.Unlock()
	return theUsage
}

func AddUsage(usage Usage) {
	theUsageLock.Lock()
	defer theUsageLock.Unlock()
	if theUsage.CompletionTokensDetails == nil {
		theUsage.CompletionTokensDetails = &CompletionTokensDetails{}
	}
	theUsage.CompletionTokens += usage.CompletionTokens
	theUsage.PromptTokens += usage.PromptTokens
	theUsage.PromptCacheHitTokens += usage.PromptCacheHitTokens
	theUsage.PromptCacheMissTokens += usage.PromptCacheMissTokens
	theUsage.TotalTokens += usage.TotalTokens
	if usage.CompletionTokensDetails != nil {
		theUsage.CompletionTokensDetails.ReasoningTokens += usage.CompletionTokensDetails.ReasoningTokens
	}
}

func GetCost(model string) (cost float64) {
	usage := GetUsage()

	prices := GetPrice()
	p, ok := prices[model]
	if !ok {
		return 0
	}

	cost = (float64(usage.PromptCacheHitTokens) / 1_000_000) * p.PromptCacheHit
	cost += (float64(usage.PromptCacheMissTokens) / 1_000_000) * p.PromptCacheMiss
	cost += (float64(usage.CompletionTokens) / 1_000_000) * p.Completion
	return cost
}
