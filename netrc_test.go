package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseNetrc(t *testing.T) {
	// 创建测试.netrc内容
	testNetrc := `machine gitcode.com
  login nanjunjie
  password test-token-123456

machine github.com
  login token
  password ghp_testtoken

# 这是一个注释行
default
  login anonymous
  password anonymous@example.com`

	// 创建临时文件
	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(testNetrc), 0o600); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	// 测试解析
	entries, err := ParseNetrc(netrcPath)
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	// 验证条目数量（github.com的条目被default打断，应该只有1个）
	if len(entries) != 1 {
		t.Errorf("条目数量不匹配: 期望=1, 实际=%d", len(entries))
		for i, entry := range entries {
			t.Logf("条目 %d: Machine=%s, Login=%s, Password=%s",
				i, entry.Machine, entry.Login, entry.Password)
		}
		return
	}

	// 验证第一个条目
	if entries[0].Machine != "gitcode.com" {
		t.Errorf("Machine不匹配: 期望=gitcode.com, 实际=%s", entries[0].Machine)
	}
	if entries[0].Login != "nanjunjie" {
		t.Errorf("Login不匹配: 期望=nanjunjie, 实际=%s", entries[0].Login)
	}
	if entries[0].Password != "test-token-123456" {
		t.Errorf("Password不匹配: 期望=test-token-123456, 实际=%s", entries[0].Password)
	}
}

func TestParseNetrcEmptyFile(t *testing.T) {
	// 测试空文件
	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(""), 0o600); err != nil {
		t.Fatalf("创建空测试文件失败: %v", err)
	}

	entries, err := ParseNetrc(netrcPath)
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	if len(entries) != 0 {
		t.Errorf("空文件应该返回0个条目, 实际=%d", len(entries))
	}
}

func TestParseNetrcWithCommentsAndEmptyLines(t *testing.T) {
	// 测试包含注释和空行的文件
	testNetrc := `# 这是一个注释

machine gitcode.com
  login user1
  password token1

# 另一个注释

machine github.com
  login user2
  password token2

`

	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(testNetrc), 0o600); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	entries, err := ParseNetrc(netrcPath)
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("条目数量不匹配: 期望=2, 实际=%d", len(entries))
	}
}

func TestGetTokenFromNetrc(t *testing.T) {
	// 保存原始HOME环境变量
	originalHome := os.Getenv("HOME")

	// 创建临时目录作为HOME
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	// 创建测试.netrc文件
	netrcContent := `machine gitcode.com
  login testuser
  password test-token-abcdef123456

machine api.gitcode.com
  login api-user
  password api-token-789012`

	netrcPath := filepath.Join(tmpHome, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(netrcContent), 0o600); err != nil {
		t.Fatalf("写入.netrc文件失败: %v", err)
	}

	// 测试获取存在的token
	token, err := GetTokenFromNetrc("gitcode.com")
	if err != nil {
		t.Fatalf("GetTokenFromNetrc错误: %v", err)
	}

	if token != "test-token-abcdef123456" {
		t.Errorf("token不匹配: 期望=test-token-abcdef123456, 实际=%s", token)
	}

	// 测试获取另一个token
	token2, err := GetTokenFromNetrc("api.gitcode.com")
	if err != nil {
		t.Fatalf("GetTokenFromNetrc错误: %v", err)
	}

	if token2 != "api-token-789012" {
		t.Errorf("token不匹配: 期望=api-token-789012, 实际=%s", token2)
	}

	// 测试不存在的host
	token3, err := GetTokenFromNetrc("nonexistent.com")
	if err != nil {
		t.Fatalf("不存在的host返回错误: %v", err)
	}
	if token3 != "" {
		t.Errorf("不存在的host应该返回空字符串，实际=%s", token3)
	}
}

func TestGetTokenFromNetrcFileNotExist(t *testing.T) {
	// 保存原始HOME环境变量
	originalHome := os.Getenv("HOME")

	// 创建临时目录作为HOME（不创建.netrc文件）
	tmpHome := t.TempDir()
	os.Setenv("HOME", tmpHome)
	defer os.Setenv("HOME", originalHome)

	// 测试不存在的.netrc文件
	token, err := GetTokenFromNetrc("gitcode.com")
	if err != nil {
		t.Fatalf("不存在的.netrc文件返回错误: %v", err)
	}
	if token != "" {
		t.Errorf("不存在的.netrc文件应该返回空字符串，实际=%s", token)
	}
}

func TestParseNetrcInvalidFormat(t *testing.T) {
	// 测试无效格式
	testNetrc := `invalid line
machine gitcode.com
login missing space
  password token`

	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(testNetrc), 0o600); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	entries, err := ParseNetrc(netrcPath)
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	// 应该只解析有效的行
	if len(entries) != 1 {
		t.Errorf("应该只解析有效的行，期望=1, 实际=%d", len(entries))
	}
}

// TestParseNetrcCompleteEntries 测试完整的条目解析
func TestParseNetrcCompleteEntries(t *testing.T) {
	// 测试两个完整的条目，没有default打断
	testNetrc := `machine gitcode.com
  login user1
  password token1

machine github.com
  login user2
  password token2`

	tmpDir := t.TempDir()
	netrcPath := filepath.Join(tmpDir, ".netrc")
	if err := os.WriteFile(netrcPath, []byte(testNetrc), 0o600); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}

	entries, err := ParseNetrc(netrcPath)
	if err != nil {
		t.Fatalf("ParseNetrc失败: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("条目数量不匹配: 期望=2, 实际=%d", len(entries))
	}
}
