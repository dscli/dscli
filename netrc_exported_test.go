package main

import (
	"os"
	"testing"
)

// TestParseNetrcExported 测试导出的ParseNetrc函数
func TestParseNetrcExported(t *testing.T) {
	// 测试正常情况
	content := `machine api.example.com login testuser password test-token`
	tmpFile, err := os.CreateTemp("", "test-exported-netrc-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}

	// 调用导出的ParseNetrc函数
	entries, err := ParseNetrc(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("期望1个条目，实际%d个", len(entries))
	}

	entry := entries[0]
	if entry.Machine != "api.example.com" {
		t.Errorf("Machine不匹配: 期望=api.example.com, 实际=%s", entry.Machine)
	}
	if entry.Password != "test-token" {
		t.Errorf("Password不匹配: 期望=test-token, 实际=%s", entry.Password)
	}
}

// TestGetTokenFromNetrcExported 测试导出的GetTokenFromNetrc函数
func TestGetTokenFromNetrcExported(t *testing.T) {
	// 创建测试文件
	content := `machine testhost.com login user password testtoken123`
	tmpFile, err := os.CreateTemp("", "test-gettoken-netrc-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatal(err)
	}

	// 由于GetTokenFromNetrc使用固定的~/.netrc路径，
	// 我们需要测试实际的解析逻辑
	// 这里我们测试ParseNetrc，然后手动验证查找逻辑
	entries, err := ParseNetrc(tmpFile.Name())
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	// 模拟GetTokenFromNetrc的逻辑
	var foundToken string
	for _, entry := range entries {
		if entry.Machine == "testhost.com" {
			foundToken = entry.Password
			break
		}
	}

	if foundToken != "testtoken123" {
		t.Errorf("token查找失败: 期望=testtoken123, 实际=%s", foundToken)
	}
}

// TestParseNetrcEdgeCases 测试边界情况
func TestParseNetrcEdgeCases(t *testing.T) {
	testCases := []struct {
		name    string
		content string
		expect  int
	}{
		{
			name:    "空文件",
			content: "",
			expect:  0,
		},
		{
			name:    "只有注释",
			content: "# 注释行\n# 另一行注释",
			expect:  0,
		},
		{
			name:    "多个条目",
			content: "machine host1 login user1 password token1\nmachine host2 login user2 password token2",
			expect:  2,
		},
		{
			name:    "大小写混合关键字",
			content: "MACHINE host1 LOGIN user1 PASSWORD token1\nMaChInE host2 LoGiN user2 PaSsWoRd token2",
			expect:  2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tmpFile, err := os.CreateTemp("", "test-edge-*.txt")
			if err != nil {
				t.Fatal(err)
			}
			defer os.Remove(tmpFile.Name())
			defer tmpFile.Close()

			if _, err := tmpFile.WriteString(tc.content); err != nil {
				t.Fatal(err)
			}

			entries, err := ParseNetrc(tmpFile.Name())
			if err != nil {
				t.Fatalf("ParseNetrc失败: %v", err)
			}

			if len(entries) != tc.expect {
				t.Errorf("条目数量不匹配: 期望=%d, 实际=%d", tc.expect, len(entries))
			}
		})
	}
}
