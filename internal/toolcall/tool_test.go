package toolcall

import (
	"testing"
)

// TestRegisterToolAndGetAllTools 测试工具注册和获取
func TestRegisterToolAndGetAllTools(t *testing.T) {
	// 测试获取工具列表
	ctx := t.Context()
	tools := GetAllTools(ctx)
	if len(tools) == 0 {
		t.Error("GetAllTools应该返回至少一个工具")
	}

	// 检查返回的Tool结构体
	for _, tool := range tools {
		if tool.Type == "" {
			t.Error("工具应该有Type字段")
		}
		if tool.Function.Name == "" {
			t.Error("工具函数应该有名称")
		}
		if tool.Function.Description == "" {
			t.Error("工具函数应该有描述")
		}
	}
}
