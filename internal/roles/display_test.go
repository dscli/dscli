package roles

import "testing"

func TestDisplayFor(t *testing.T) {
	tests := []struct {
		role string
		want Display
	}{
		{"dev", devDisplay},
		{"expert", expertDisplay},
		{"review", reviewDisplay},
		{"test", testDisplay},
		{"architect", architectDisplay},
		{"", Display{}},
		{"unknown", devDisplay},
	}
	for _, tt := range tests {
		if got := DisplayFor(tt.role); got != tt.want {
			t.Errorf("DisplayFor(%q) = %+v, want %+v", tt.role, got, tt.want)
		}
	}
}

func TestDisplayString(t *testing.T) {
	if got := reviewDisplay.String(); got != "review·代码审查" {
		t.Errorf("reviewDisplay.String() = %q, want %q", got, "review·代码审查")
	}
	if got := (Display{}).String(); got != "" {
		t.Errorf("zero Display.String() = %q, want empty", got)
	}
}

func TestDisplayName(t *testing.T) {
	if got := DisplayName(""); got != "专家" {
		t.Errorf("DisplayName(\"\") = %q, want 专家", got)
	}
	if got := DisplayName("review"); got != "review·代码审查" {
		t.Errorf("DisplayName(\"review\") = %q, want review·代码审查", got)
	}
	// 未知/未识别角色回退 dev 的组合名（与 DefaultFor 策略一致）。
	if got := DisplayName("bogus"); got != "dev·开发助手" {
		t.Errorf("DisplayName(\"bogus\") = %q, want dev·开发助手", got)
	}
}
