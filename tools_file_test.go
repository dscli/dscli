package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHandleReadFile 测试读取文件功能
func TestHandleReadFile(t *testing.T) {
	// 创建测试上下文
	ctx := context.Background()
	
	// 创建测试文件
	testContent := `测试文件内容：
1. 第一行
2. 第二行
3. 特殊字符: \` + "`echo test`" + ` $PATH "quotes" 'single'
4. 中文: 你好世界！`
	
	testFileName := "test_read_file.txt"
	testFilePath := filepath.Join(ProjectRoot, testFileName)
	
	// 确保清理
	defer os.Remove(testFilePath)
	
	// 创建测试文件
	if err := os.WriteFile(testFilePath, []byte(testContent), 0644); err != nil {
		t.Fatalf("创建测试文件失败: %v", err)
	}
	
	// 测试用例
	testCases := []struct {
		name     string
		path     string
		expected string
		wantErr  bool
	}{
		{
			name:     "读取存在的文件",
			path:     testFileName,
			expected: testContent,
			wantErr:  false,
		},
		{
			name:     "读取不存在的文件",
			path:     "non_existent_file.txt",
			expected: "",
			wantErr:  true,
		},
		{
			name:     "相对路径",
			path:     "./" + testFileName,
			expected: testContent,
			wantErr:  false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 准备参数
			args := map[string]string{
				"path": tc.path,
			}
			argsJSON, err := json.Marshal(args)
			if err != nil {
				t.Fatalf("序列化参数失败: %v", err)
			}
			
			// 调用handleReadFile
			result, err := handleReadFile(ctx, argsJSON)
			
			// 检查错误
			if tc.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到成功")
				}
				return
			}
			
			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}
			
			// 检查结果
			if !strings.Contains(result, tc.expected) {
				t.Errorf("结果不包含期望内容\n期望包含: %s\n实际结果: %s", 
					tc.expected, result)
			}
			
			// 检查输出格式
			if !strings.Contains(result, "=== 执行结果 ===") {
				t.Errorf("结果缺少执行结果部分")
			}
			if !strings.Contains(result, "=== 执行统计 ===") {
				t.Errorf("结果缺少执行统计部分")
			}
		})
	}
}

// TestHandleWriteFile 测试写入文件功能
func TestHandleWriteFile(t *testing.T) {
	// 创建测试上下文
	ctx := context.Background()
	
	// 测试用例
	testCases := []struct {
		name    string
		path    string
		content string
		wantErr bool
	}{
		{
			name:    "写入普通文本",
			path:    "test_write_normal.txt",
			content: "普通文本内容",
			wantErr: false,
		},
		{
			name:    "写入特殊字符",
			path:    "test_write_special.txt",
			content: "特殊字符: \`echo\` $PATH \"quotes\" 'single'",
			wantErr: false,
		},
		{
			name:    "写入多行内容",
			path:    "test_write_multiline.txt",
			content: "第一行\n第二行\n第三行",
			wantErr: false,
		},
		{
			name:    "写入中文",
			path:    "test_write_chinese.txt",
			content: "中文测试：你好世界！",
			wantErr: false,
		},
		{
			name:    "写入空内容",
			path:    "test_write_empty.txt",
			content: "",
			wantErr: false,
		},
		{
			name:    "创建目录并写入",
			path:    "test_dir/subdir/test_file.txt",
			content: "嵌套目录测试",
			wantErr: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// 准备参数
			args := map[string]string{
				"path":    tc.path,
				"content": tc.content,
			}
			argsJSON, err := json.Marshal(args)
			if err != nil {
				t.Fatalf("序列化参数失败: %v", err)
			}
			
			// 调用handleWriteFile
			result, err := handleWriteFile(ctx, argsJSON)
			
			// 检查错误
			if tc.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到成功")
				}
				return
			}
			
			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}
			
			// 验证文件是否创建
			fullPath := filepath.Join(ProjectRoot, tc.path)
			fileContent, err := os.ReadFile(fullPath)
			if err != nil {
				t.Errorf("读取写入的文件失败: %v", err)
				return
			}
			
			// 验证内容
			if string(fileContent) != tc.content {
				t.Errorf("文件内容不匹配\n期望: %q\n实际: %q", 
					tc.content, string(fileContent))
			}
			
			// 检查输出格式
			if !strings.Contains(result, "=== 执行结果 ===") {
				t.Errorf("结果缺少执行结果部分")
			}
			if !strings.Contains(result, "=== 执行统计 ===") {
				t.Errorf("结果缺少执行统计部分")
			}
			if !strings.Contains(result, tc.path) {
				t.Errorf("结果不包含文件路径")
			}
			
			// 清理
			os.Remove(fullPath)
			// 清理可能创建的目录
			os.RemoveAll(filepath.Dir(fullPath))
		})
	}
}

// TestReadWriteIntegration 测试读写集成
func TestReadWriteIntegration(t *testing.T) {
	ctx := context.Background()
	
	// 测试内容
	testContent := `集成测试内容：
特殊字符: \` + "`echo test`" + `
环境变量: $HOME
引号: "double" 'single'
换行:
  第二行
  第三行
中文: 你好世界！`
	
	testFileName := "test_integration.txt"
	
	// 先写入文件
	writeArgs := map[string]string{
		"path":    testFileName,
		"content": testContent,
	}
	writeArgsJSON, _ := json.Marshal(writeArgs)
	
	_, err := handleWriteFile(ctx, writeArgsJSON)
	if err != nil {
		t.Fatalf("写入文件失败: %v", err)
	}
	defer os.Remove(filepath.Join(ProjectRoot, testFileName))
	
	// 再读取文件
	readArgs := map[string]string{
		"path": testFileName,
	}
	readArgsJSON, _ := json.Marshal(readArgs)
	
	readResult, err := handleReadFile(ctx, readArgsJSON)
	if err != nil {
		t.Fatalf("读取文件失败: %v", err)
	}
	
	// 验证读取的内容包含写入的内容
	if !strings.Contains(readResult, testContent) {
		t.Errorf("读取的内容不包含写入的内容")
	}
}

// TestResolvePath 测试路径解析
func TestResolvePath(t *testing.T) {
	testCases := []struct {
		name     string
		path     string
		wantErr  bool
		checkInProject bool
	}{
		{
			name:     "相对路径",
			path:     "main.go",
			wantErr:  false,
			checkInProject: true,
		},
		{
			name:     "当前目录",
			path:     "./main.go",
			wantErr:  false,
			checkInProject: true,
		},
		{
			name:     "上级目录",
			path:     "../",
			wantErr:  true, // 不在项目内
			checkInProject: false,
		},
		{
			name:     "绝对路径在项目内",
			path:     filepath.Join(ProjectRoot, "main.go"),
			wantErr:  false,
			checkInProject: true,
		},
		{
			name:     "绝对路径不在项目内",
			path:     "/tmp/outside.txt",
			wantErr:  true,
			checkInProject: false,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := resolvePath(tc.path)
			
			if tc.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到成功，结果: %s", result)
				}
				return
			}
			
			if err != nil {
				t.Errorf("不期望的错误: %v", err)
				return
			}
			
			// 检查是否在项目内
			if tc.checkInProject {
				rel, err := filepath.Rel(ProjectRoot, result)
				if err != nil || strings.HasPrefix(rel, "..") {
					t.Errorf("路径不在项目内: %s", result)
				}
			}
		})
	}
}
