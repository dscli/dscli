package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Editor 工具定义
var EditorTool = ToolDef{
	Name:        "editor",
	DisplayName: "编辑器",
	Description: "打开编辑器让用户编辑内容，返回编辑后的文本",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{
				"type":        "string",
				"description": "显示给用户的提示信息（可选）",
			},
			"initial_content": map[string]any{
				"type":        "string",
				"description": "编辑器的初始内容（可选）",
			},
		},
		"required": []string{},
	},
	Category: "interaction",
	Timeout:  300 * time.Second, // 给用户5分钟时间编辑
	Handler:  handleEditor,
}

func init() {
	RegisterTool(EditorTool)
}

// handleEditor 处理编辑器工具调用
func handleEditor(ctx context.Context, args map[string]string) (string, error) {
	prompt := args["prompt"]
	initialContent := args["initial_content"]

	// 如果有提示信息，先显示
	if prompt != "" {
		fmt.Printf("\n%s\n\n", prompt)
	}

	// 调用编辑器
	content, err := OpenEditor(initialContent)
	if err != nil {
		return "", fmt.Errorf("编辑器错误: %v", err)
	}

	return string(content), nil
}

// OpenEditor 打开编辑器让用户编辑内容
func OpenEditor(initialContent string) ([]byte, error) {
	// 检查是否应该使用Emacs内置编辑器
	if os.Getenv("DS_CLI_USE_EMACS_EDITOR") != "" {
		// 使用Emacs内置编辑器
		return openEditorInEmacs(initialContent)
	}

	// 检测编辑器
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
		if editor == "" {
			// 尝试常见编辑器
			for _, p := range []string{"editor", "vi", "vim", "emacs", "nano", "code", "subl"} {
				_, err := exec.LookPath(p)
				if err == nil {
					editor = p
					break
				}
			}
			if editor == "" {
				return nil, errors.New("未找到文本编辑器，请设置 EDITOR 环境变量")
			}
		}
	}

	// 创建临时文件
	tmpFile, err := os.CreateTemp("", "dscli_editor_*.txt")
	if err != nil {
		return nil, fmt.Errorf("创建临时文件失败: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// 写入初始内容
	if initialContent != "" {
		if _, err := tmpFile.WriteString(initialContent); err != nil {
			tmpFile.Close()
			return nil, fmt.Errorf("写入初始内容失败: %v", err)
		}
	}

	// 关闭文件以便编辑器可以访问
	if err := tmpFile.Close(); err != nil {
		return nil, fmt.Errorf("关闭临时文件失败: %v", err)
	}

	// 设置文件权限
	if err := os.Chmod(tmpFile.Name(), 0o600); err != nil {
		return nil, fmt.Errorf("设置文件权限失败: %v", err)
	}

	// 调用编辑器
	cmdParts := strings.Fields(editor)
	cmd := exec.Command(cmdParts[0], append(cmdParts[1:], tmpFile.Name())...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("编辑器执行失败: %v", err)
	}

	// 读取编辑后的内容
	content, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		return nil, fmt.Errorf("读取编辑内容失败: %v", err)
	}

	return content, nil
}

// openEditorInEmacs 在Emacs环境中打开编辑器
func openEditorInEmacs(initialContent string) ([]byte, error) {
	// 输出特殊标记，告诉Emacs需要打开编辑器
	fmt.Println("<!-- DS-CLI-EDITOR-START -->")

	// 转义初始内容中的特殊字符，防止HTML注释被意外关闭
	escapedContent := strings.ReplaceAll(initialContent, "-->", "->")
	// 同时转义换行符，确保内容在一行内
	escapedContent = strings.ReplaceAll(escapedContent, "\n", "\\n")
	escapedContent = strings.ReplaceAll(escapedContent, "\r", "\\r")

	fmt.Printf("<!-- DS-CLI-EDITOR-CONTENT:%s -->\n", escapedContent)
	fmt.Println("<!-- DS-CLI-EDITOR-END -->")

	// 等待Emacs返回编辑结果
	// 这里需要从标准输入读取Emacs返回的内容
	reader := bufio.NewReader(os.Stdin)

	// 设置读取超时（30秒）
	// 注意：标准输入通常不支持超时，这里我们使用一个简单的循环
	contentChan := make(chan string)
	errorChan := make(chan error)

	go func() {
		content, err := reader.ReadString('\x00') // 使用空字符作为分隔符
		if err != nil && err != io.EOF {
			errorChan <- fmt.Errorf("读取Emacs编辑内容失败: %v", err)
			return
		}
		contentChan <- content
	}()

	select {
	case content := <-contentChan:
		// 移除分隔符
		if len(content) > 0 && content[len(content)-1] == '\x00' {
			content = content[:len(content)-1]
		}
		// 恢复转义的换行符
		content = strings.ReplaceAll(content, "\\n", "\n")
		content = strings.ReplaceAll(content, "\\r", "\r")
		return []byte(content), nil

	case err := <-errorChan:
		return nil, err

	case <-time.After(30 * time.Second):
		return nil, errors.New("等待Emacs编辑超时（30秒）")
	}
}
