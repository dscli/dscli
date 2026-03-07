package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

// EditRequest 表示编辑请求
type EditRequest struct {
	FilePath    string `json:"file_path"`
	Language    string `json:"language"`
	Instruction string `json:"instruction"`
	Context     string `json:"context,omitempty"`
	Target      string `json:"target,omitempty"` // 目标函数/类名
	LineRange   string `json:"line_range,omitempty"`
}

// EditResponse 表示编辑响应
type EditResponse struct {
	Success     bool   `json:"success"`
	Original    string `json:"original,omitempty"`
	Modified    string `json:"modified,omitempty"`
	Explanation string `json:"explanation,omitempty"`
	Error       string `json:"error,omitempty"`
}

func init() {
	editCmd := &cobra.Command{
		Use:   "edit <file>",
		Short: "LLM-assisted file editing",
		Long: `LLM-assisted file editing with context awareness.
This command analyzes file structure and provides intelligent editing suggestions.

Examples:
  # Edit a Go file with instruction
  dscli edit main.go --instruction "Add error handling to the main function"
  
  # Edit specific function
  dscli edit main.go --target "parseFile" --instruction "Add logging"
  
  # Edit with line range
  dscli edit main.go --lines "10-20" --instruction "Refactor this code"
  
  # Edit with context from another file
  dscli edit main.go --context "utils.go" --instruction "Make consistent with utils.go"`,
		Args: cobra.ExactArgs(1),
		RunE: runEdit,
	}

	// 添加选项
	editCmd.Flags().StringP("instruction", "i", "", "Editing instruction (required)")
	editCmd.Flags().StringP("target", "t", "", "Target function/class name")
	editCmd.Flags().StringP("lines", "l", "", "Line range (e.g., '10-20')")
	editCmd.Flags().StringP("context", "c", "", "Context file for reference")
	editCmd.Flags().StringP("language", "L", "", "Specify language (auto-detected by default)")
	editCmd.Flags().BoolP("dry-run", "d", false, "Dry run - show what would be changed")
	editCmd.Flags().BoolP("interactive", "I", false, "Interactive mode")
	editCmd.Flags().BoolP("verbose", "v", false, "Verbose output")

	editCmd.MarkFlagRequired("instruction")

	AddRootCommand(editCmd)
}

// runEdit 是 edit 子命令的入口函数
func runEdit(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// 获取选项
	instruction, _ := cmd.Flags().GetString("instruction")
	target, _ := cmd.Flags().GetString("target")
	lineRange, _ := cmd.Flags().GetString("lines")
	contextFile, _ := cmd.Flags().GetString("context")
	language, _ := cmd.Flags().GetString("language")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	interactive, _ := cmd.Flags().GetBool("interactive")
	verbose, _ := cmd.Flags().GetBool("verbose")

	if language == "" {
		language = guessLanguage(filePath)
	}

	// 读取文件内容
	content, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// 获取文件结构
	fs, err := parseFileStructure(cmd.Context(), filePath, language, verbose, false)
	if err != nil {
		return fmt.Errorf("failed to parse file structure: %w", err)
	}

	// 读取上下文文件（如果有）
	var contextContent string
	if contextFile != "" {
		ctxContent, err := os.ReadFile(contextFile)
		if err != nil {
			return fmt.Errorf("failed to read context file: %w", err)
		}
		contextContent = string(ctxContent)
	}

	// 构建编辑请求
	editReq := EditRequest{
		FilePath:    filePath,
		Language:    language,
		Instruction: instruction,
		Context:     contextContent,
		Target:      target,
		LineRange:   lineRange,
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "File: %s\n", filePath)
		fmt.Fprintf(os.Stderr, "Language: %s\n", language)
		fmt.Fprintf(os.Stderr, "Target: %s\n", target)
		fmt.Fprintf(os.Stderr, "Line range: %s\n", lineRange)
		fmt.Fprintf(os.Stderr, "Instruction: %s\n", instruction)
		fmt.Fprintf(os.Stderr, "File structure: %d functions, %d classes, %d imports\n",
			len(fs.Functions), len(fs.Classes), len(fs.Imports))
	}

	// 如果是dry-run模式，只显示分析结果
	if dryRun {
		fmt.Println("=== DRY RUN ===")
		fmt.Printf("File: %s\n", filePath)
		fmt.Printf("Language: %s\n", language)
		fmt.Printf("Instruction: %s\n", instruction)

		if target != "" {
			fmt.Printf("Target: %s\n", target)
			// 查找目标符号
			found := false
			for _, f := range fs.Functions {
				if f.Name == target {
					fmt.Printf("Found function: %s (lines %d-%d)\n", f.Name, f.Line, f.EndLine)
					found = true
					break
				}
			}
			for _, c := range fs.Classes {
				if c.Name == target {
					fmt.Printf("Found class: %s (lines %d-%d)\n", c.Name, c.Line, c.EndLine)
					found = true
					break
				}
			}
			if !found {
				fmt.Printf("Warning: Target '%s' not found in file\n", target)
			}
		}

		fmt.Println("\nWould perform edit with LLM assistance")
		return nil
	}

	// 准备LLM请求
	return performLLMEdit(cmd.Context(), editReq, string(content), fs, interactive, verbose)
}

// performLLMEdit 执行LLM辅助编辑
func performLLMEdit(ctx context.Context, req EditRequest, content string, fs *FileStructure, interactive, verbose bool) error {
	// 构建系统提示
	systemPrompt := buildEditSystemPrompt(req, fs)

	// 构建用户消息
	userMessage := buildEditUserMessage(req, content)

	if verbose {
		fmt.Fprintf(os.Stderr, "=== System Prompt ===\n%s\n\n", systemPrompt)
		fmt.Fprintf(os.Stderr, "=== User Message ===\n%s\n\n", userMessage)
	}

	// 调用LLM API
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	} // 使用聊天功能
	resp, err := DeepseekClient.Chat(ModelDeepseekChat, messages, nil)
	if err != nil {
		return fmt.Errorf("LLM API error: %w", err)
	}

	// 解析LLM响应
	editResp, err := parseLLMResponse(resp.Choices[0].Message.Content)
	if err != nil {
		return fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if !editResp.Success {
		return fmt.Errorf("LLM edit failed: %s", editResp.Error)
	}

	// 显示结果
	fmt.Println("=== Edit Result ===")
	if editResp.Explanation != "" {
		fmt.Printf("Explanation: %s\n\n", editResp.Explanation)
	}

	if interactive {
		// 交互式模式：显示差异并询问确认
		fmt.Println("=== Original ===")
		fmt.Println(editResp.Original)
		fmt.Println("\n=== Modified ===")
		fmt.Println(editResp.Modified)

		// TODO: 添加交互式确认逻辑
		fmt.Println("\nWould you like to apply these changes? (y/n)")
		// 这里可以添加实际的交互逻辑
	} else {
		// 非交互式模式：直接显示修改后的内容
		fmt.Println(editResp.Modified)
	}

	return nil
}

// buildEditSystemPrompt 构建编辑系统提示
func buildEditSystemPrompt(req EditRequest, fs *FileStructure) string {
	var sb strings.Builder

	sb.WriteString("You are an expert code editor. Your task is to edit code based on the given instruction.\n\n")

	sb.WriteString("File Information:\n")
	sb.WriteString(fmt.Sprintf("- File: %s\n", req.FilePath))
	sb.WriteString(fmt.Sprintf("- Language: %s\n", req.Language))

	if req.Target != "" {
		sb.WriteString(fmt.Sprintf("- Target: %s\n", req.Target))
	}

	if req.LineRange != "" {
		sb.WriteString(fmt.Sprintf("- Line range: %s\n", req.LineRange))
	}

	sb.WriteString("\nFile Structure:\n")

	// 添加包信息（如果是Go）
	if fs.Package != "" {
		sb.WriteString(fmt.Sprintf("- Package: %s\n", fs.Package))
	}

	// 添加导入
	if len(fs.Imports) > 0 {
		sb.WriteString("- Imports:\n")
		for _, imp := range fs.Imports {
			sb.WriteString(fmt.Sprintf("  - %s\n", imp))
		}
	}

	// 添加函数
	if len(fs.Functions) > 0 {
		sb.WriteString("- Functions:\n")
		for _, f := range fs.Functions {
			lineInfo := fmt.Sprintf("lines %d-%d", f.Line, f.EndLine)
			if f.Receiver != "" {
				sb.WriteString(fmt.Sprintf("  - %s.%s (%s) %s\n", f.Receiver, f.Name, f.Type, lineInfo))
			} else {
				sb.WriteString(fmt.Sprintf("  - %s (%s) %s\n", f.Name, f.Type, lineInfo))
			}
		}
	}

	// 添加类
	if len(fs.Classes) > 0 {
		sb.WriteString("- Classes/Structs/Interfaces:\n")
		for _, c := range fs.Classes {
			sb.WriteString(fmt.Sprintf("  - %s (%s) lines %d-%d\n", c.Name, c.Type, c.Line, c.EndLine))
		}
	}

	sb.WriteString("\nEditing Rules:\n")
	sb.WriteString("1. Only make changes specified by the instruction\n")
	sb.WriteString("2. Preserve code style and formatting\n")
	sb.WriteString("3. Maintain backward compatibility unless specified\n")
	sb.WriteString("4. Add appropriate comments for significant changes\n")
	sb.WriteString("5. Follow language-specific best practices\n")

	sb.WriteString("\nOutput Format:\n")
	sb.WriteString("Respond with a JSON object containing:\n")
	sb.WriteString("- success: boolean indicating if edit was successful\n")
	sb.WriteString("- original: the original code section (if applicable)\n")
	sb.WriteString("- modified: the modified code section\n")
	sb.WriteString("- explanation: brief explanation of changes\n")
	sb.WriteString("- error: error message if failed\n")

	return sb.String()
}

// buildEditUserMessage 构建用户消息
func buildEditUserMessage(req EditRequest, content string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Instruction: %s\n\n", req.Instruction))

	if req.Context != "" {
		sb.WriteString("=== Context File Content ===\n")
		sb.WriteString(req.Context)
		sb.WriteString("\n\n")
	}

	sb.WriteString("=== File Content ===\n")
	sb.WriteString(content)

	return sb.String()
}

// parseLLMResponse 解析LLM响应
func parseLLMResponse(response string) (*EditResponse, error) {
	// 尝试解析JSON响应
	var editResp EditResponse
	if err := json.Unmarshal([]byte(response), &editResp); err == nil {
		return &editResp, nil
	}

	// 如果不是有效的JSON，尝试提取代码块
	lines := strings.Split(response, "\n")
	var inCodeBlock bool
	var codeBlock []string

	for _, line := range lines {
		if strings.Contains(line, "```") {
			if inCodeBlock {
				inCodeBlock = false
			} else {
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			codeBlock = append(codeBlock, line)
		}
	}

	if len(codeBlock) > 0 {
		return &EditResponse{
			Success:     true,
			Modified:    strings.Join(codeBlock, "\n"),
			Explanation: "Code extracted from response",
		}, nil
	}

	// 如果既不是JSON也没有代码块，返回整个响应作为修改
	return &EditResponse{
		Success:     true,
		Modified:    response,
		Explanation: "Full response used as modified code",
	}, nil
}
