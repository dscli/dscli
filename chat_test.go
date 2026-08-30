package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/toolcall"
)

func TestChatRoleFlagDefaultsToArchitect(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"chat"})
	if err != nil {
		t.Fatalf("find chat command: %v", err)
	}
	if cmd == nil {
		t.Fatal("chat command not found")
	}
	flag := cmd.Flags().Lookup("role")
	if flag == nil {
		t.Fatal("--role flag not registered")
	}
	if flag.DefValue != defaultChatRole {
		t.Errorf("default --role = %q, want %q", flag.DefValue, defaultChatRole)
	}

	// Restore the shared command's role flag after the test so mutations do
	// not leak into other top-level tests in the package (order-dependent
	// under -shuffle=on). Restore to the canonical DefValue, not the captured
	// current value, so even a pre-polluted flag is reset.
	t.Cleanup(func() {
		if err := cmd.Flags().Set("role", flag.DefValue); err != nil {
			t.Errorf("restore --role in cleanup: %v", err)
		}
	})

	tests := []struct {
		name    string
		set     bool
		roleArg string
		want    string
	}{
		{name: "unset flag returns registered default", want: defaultChatRole},
		{name: "empty role falls back to default", set: true, roleArg: "", want: defaultChatRole},
		{name: "explicit role is not overridden", set: true, roleArg: "expert", want: "expert"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset the shared command's role flag to a deterministic value
			// so each subtest starts from a known state regardless of order.
			roleArg := flag.DefValue
			if tt.set {
				roleArg = tt.roleArg
			}
			if err := cmd.Flags().Set("role", roleArg); err != nil {
				t.Fatalf("set --role: %v", err)
			}
			cmd.SetContext(t.Context())
			if err := ChatPreRunE(cmd, nil); err != nil {
				t.Fatalf("ChatPreRunE: %v", err)
			}
			got := context.ContextValue(cmd.Context(), context.CurrentRoleKey, "")
			if got != tt.want {
				t.Errorf("role = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintContent(t *testing.T) {
	ctx := t.Context()
	ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())
	// make sure two keys  no overlap
	ctx = context.WithValue(ctx, context.CurrentModelIDKey, context.ModelDeepseekChat)
	buf := bytes.NewBuffer([]byte{})
	outfmt.SetOutputWriter(buf)
	t.Cleanup(func() { outfmt.SetOutputWriter(os.Stdout) })
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

func TestPrintContentRoleHeader(t *testing.T) {
	buf := bytes.NewBuffer([]byte{})
	outfmt.SetOutputWriter(buf)
	t.Cleanup(func() { outfmt.SetOutputWriter(os.Stdout) })

	tests := []struct {
		name      string
		role      string
		cn        string
		email     string
		birdFrog  string
		reasoning string
		content   string
		want      string
		want2     string
		notWant   string
	}{
		{
			name:      "review role",
			role:      "review",
			reasoning: "thinking",
			content:   "answer",
			want:      "review·代码审查",
		},
		{
			name:      "architect default role, reasoning only",
			role:      "architect",
			reasoning: "thinking",
			want:      "architect·软件架构师",
		},
		{
			name:      "architect role with AI name",
			role:      "architect",
			cn:        "玻尔",
			email:     "bohr@dscli.io",
			birdFrog:  "bird",
			reasoning: "thinking",
			content:   "answer",
			want:      "玻尔 <bohr@dscli.io>",
			want2:     "architect·软件架构师",
		},
		{
			name:      "role with name but no email falls back to role header",
			role:      "architect",
			cn:        "玻尔",
			birdFrog:  "bird",
			reasoning: "thinking",
			content:   "answer",
			want:      "architect·软件架构师",
			notWant:   "玻尔",
		},
		{
			name:      "unknown role falls back to dev",
			role:      "bogus",
			reasoning: "thinking",
			content:   "answer",
			want:      "dev·开发助手",
		},
		{
			name:      "no role uses AI name",
			cn:        "玻尔",
			email:     "bohr@dscli.io",
			birdFrog:  "bird",
			reasoning: "thinking",
			content:   "answer",
			want:      "玻尔",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			ctx := t.Context()
			ctx = context.WithValue(ctx, context.StartTimeKey, time.Now())
			if tt.role != "" {
				ctx = context.WithValue(ctx, context.CurrentRoleKey, tt.role)
			}
			if tt.cn != "" {
				ctx = context.WithValue(ctx, context.AINameCNKey, tt.cn)
				ctx = context.WithValue(ctx, context.AINameEmailKey, tt.email)
				ctx = context.WithValue(ctx, context.AINameBirdFrogKey, tt.birdFrog)
			}
			outfmt.PrintContent(ctx, tt.reasoning, tt.content)
			s := buf.String()

			if !strings.Contains(s, tt.want) {
				t.Errorf("header missing %q, got:\n%s", tt.want, s)
			}
			if tt.want2 != "" && !strings.Contains(s, tt.want2) {
				t.Errorf("header missing %q, got:\n%s", tt.want2, s)
			}
			if tt.notWant != "" && strings.Contains(s, tt.notWant) {
				t.Errorf("header should not contain %q, got:\n%s", tt.notWant, s)
			}
			if strings.Contains(s, "T:") {
				t.Errorf("header should not contain T:, got:\n%s", s)
			}
		})
	}
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
		"⏳ 30.0s",
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

	if !strings.Contains(output, "⚠️ ") {
		t.Errorf("低余额时应该显示提醒\n完整输出:\n%s", output)
	}
}

// MockDeepseekClient 用于测试的模拟客户端
type MockDeepseekClient struct {
	balanceResponse *dsc.BalanceResponse
	balanceError    error
}

func (m *MockDeepseekClient) Balance(ctx context.Context) (*dsc.BalanceResponse, error) {
	return m.balanceResponse, m.balanceError
}

func (m *MockDeepseekClient) Models(ctx context.Context) (*dsc.ModelsResponse, error) {
	return nil, nil
}

func (m *MockDeepseekClient) FIM(ctx context.Context, req dsc.FIMRequest) (*dsc.FIMResponse, error) {
	return nil, nil
}

func (m *MockDeepseekClient) Chat(ctx context.Context, messages []prompt.Message, tools []toolcall.Tool) (*dsc.ChatResponse, error) {
	return nil, nil
}
