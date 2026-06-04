package price

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var thePrice map[string]Price
var thePriceOnce sync.Once

type Price struct {
	PromptCacheHit  float64 `json:"prompt_cache_hit,omitzero"`
	PromptCacheMiss float64 `json:"prompt_cache_miss,omitzero"`
	Completion      float64 `json:"completion,omitzero"`
}

func GetPrice() (price map[string]Price) {
	thePriceOnce.Do(func() {
		resp, err := http.Get("https://api-docs.deepseek.com/zh-cn/quick_start/pricing")
		if err != nil {
			thePriceOnce = sync.Once{}
			return
		}
		body := resp.Body
		defer body.Close()
		b, err := io.ReadAll(body)
		if err != nil {
			thePriceOnce = sync.Once{}
		}
		thePrice = parsePrice(string(b))
	})
	return thePrice
}

func parsePrice(html string) (price map[string]Price) {
	flash := Price{}
	pro := Price{}
	_, after, found := strings.Cut(html, ">价格</td><td>")
	if !found {
		return
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
