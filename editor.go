package main

import (
	"context"
	"errors"
	"fmt"
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
