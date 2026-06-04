package price

import (
	"fmt"
	"reflect"
	"testing"
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
