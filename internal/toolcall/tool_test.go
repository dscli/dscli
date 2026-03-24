package toolcall

import (
	"testing"
)

// TestRegisterToolAndGetAllTools 测试工具注册和获取
func TestRegisterToolAndGetAllTools(t *testing.T) {
	// 测试获取工具列表
	ctx := t.Context()
	tools := GetAllTools(ctx)
	if len(tools) != 0 {
		t.Error("工具不应存在于工具框架中")
	}
}
