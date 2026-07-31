package price

import (
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

var (
	thePrice   map[string]Price
	thePriceMu sync.Mutex
)

type Price struct {
	PromptCacheHit  float64 `json:"prompt_cache_hit,omitzero"`
	PromptCacheMiss float64 `json:"prompt_cache_miss,omitzero"`
	Completion      float64 `json:"completion,omitzero"`
}

func GetPrice() (price map[string]Price) {
	thePriceMu.Lock()
	defer thePriceMu.Unlock()
	if thePrice != nil {
		return thePrice
	}

	resp, err := http.Get("https://api-docs.deepseek.com/zh-cn/quick_start/pricing")
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	thePrice = parsePrice(string(b))
	return thePrice
}

func parsePrice(html string) (price map[string]Price) {
	flash := Price{}
	pro := Price{}
	_, after, found := strings.Cut(html, ">价格</td><td>")
	if !found {
		// 2026-07 页面改版后，"价格"单元格带脚注标注：
		//   <td rowspan="3">价格<sup>(2)</sup></td><td>百万tokens输入（缓存命中）</td><td>0.02元</td>...
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
