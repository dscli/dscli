package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
)

func TestPrintContent(t *testing.T) {
	ctx := t.Context()
	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())
	// make sure two keys  no overlap
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, context.ModelDeepseekChat)
	buf := bytes.NewBuffer([]byte{})
	outfmt.SetOutputWriter(buf)
	outfmt.PrintContent(ctx, "reasoning", "content")
	s := buf.String()

	// 检查输出是否包含 reasoning 和 content
	if !strings.Contains(s, "reasoning") {
		t.Error("missing reasoning")
	}
	if !strings.Contains(s, "content") {
		t.Error("missing content")
	}
	// 注意：PrintContent 函数本身不输出执行时间
	// 执行时间是在 PrintSessionStats 中输出的
	// 所以这里不应该检查执行时间
}

func TestPrintToolCalls(t *testing.T) {
}

func TestPrintSessionStats(t *testing.T) {
	ctx := t.Context()
	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now().Add(-30*time.Second))

	// 设置起始余额
	startBalance := map[string]string{
		"currency":      "CNY",
		"total_balance": "100.00",
	}
	ctx = context.WithValue(ctx, context.StartBalanceKey, startBalance)

	// 模拟DeepseekClient.Balance响应
	originalClient := DeepseekClient
	defer func() { DeepseekClient = originalClient }()

	// 创建模拟客户端
	mockClient := &MockDeepseekClient{
		balanceResponse: &dsc.BalanceResponse{
			BalanceInfos: []map[string]string{
				{
					"currency":      "CNY",
					"total_balance": "95.50", // 模拟花费4.5元后的余额
				},
			},
		},
	}
	DeepseekClient = mockClient

	// 捕获输出
	buf := bytes.NewBuffer([]byte{})
	outfmt.SetOutputWriter(buf)

	// 调用函数
	PrintSessionStats(ctx)

	output := buf.String()

	// 检查输出是否包含期望的内容
	// 注意：💰 花费现在由 price.GetCost 计算，需要价格数据和 usage 累积。
	// 在单元测试环境中没有价格数据，因此不验证具体花费金额。
	expectedStrings := []string{
		"⏱️ 30.0s",
		"💳 CNY 95.50",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("输出中缺少: %s\n完整输出:\n%s", expected, output)
		}
	}

	// 测试低余额提醒
	lowBalanceClient := &MockDeepseekClient{
		balanceResponse: &dsc.BalanceResponse{
			BalanceInfos: []map[string]string{
				{
					"currency":      "CNY",
					"total_balance": "5.00", // 低于10元
				},
			},
		},
	}
	DeepseekClient = lowBalanceClient

	buf.Reset()
	PrintSessionStats(ctx)
	output = buf.String()

	if !strings.Contains(output, "⚠️ 余额较低，请及时充值！") {
		t.Errorf("低余额时应该显示提醒\n完整输出:\n%s", output)
	}
}

// MockDeepseekClient 用于测试的模拟客户端
type MockDeepseekClient struct {
	balanceResponse *dsc.BalanceResponse
	balanceError    error
}

func (m *MockDeepseekClient) Balance() (*dsc.BalanceResponse, error) {
	return m.balanceResponse, m.balanceError
}

func (m *MockDeepseekClient) Models() (*dsc.ModelsResponse, error) {
	return nil, nil
}

func (m *MockDeepseekClient) FIM(ctx context.Context, req dsc.FIMRequest) (*dsc.FIMResponse, error) {
	return nil, nil
}

func (m *MockDeepseekClient) Chat(ctx context.Context, messages []prompt.Message, tools []toolcall.Tool) (*dsc.ChatResponse, error) {
	return nil, nil
}
