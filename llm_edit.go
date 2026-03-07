package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/spf13/cobra"
)

// LLMEditRequest 表示LLM编辑请求
type LLMEditRequest struct {
	FilePath    string `json:"file_path"`
	Language    string `json:"language"`
	Instruction string `json:"instruction"`
	Target      string `json:"target,omitempty"`  // 目标函数/类名
	Context     string `json:"context,omitempty"` // 上下文文件内容
}

// LLMEditResponse 表示LLM编辑响应
type LLMEditResponse struct {
	Success     bool   `json:"success"`
	OldText     string `json:"old_text,omitempty"`
	NewText     string `json:"new_text,omitempty"`
	Explanation string `json:"explanation,omitempty"`
	Error       string `json:"error,omitempty"`
}

func init() {
	llmEditCmd := &cobra.Command{
		Use:   "llm-edit <file>",
		Short: "LLM-assisted file editing using content-based approach",
		Long: `LLM-assisted file editing using content-based approach (no line numbers).
This command follows the design principles from docs/llm_editor.org:
- Content-based matching instead of line numbers
- Declarative modifications
- Structure-aware editing

Examples:
  # Edit a Go file with instruction
  dscli llm-edit main.go --instruction "Add error handling to the main function"
  
  # Edit specific function
  dscli llm-edit main.go --target "parseFile" --instruction "Add logging"
  
  # Edit with context from another file
  dscli llm-edit main.go --context "utils.go" --instruction "Make consistent with utils.go"`,
		Args: cobra.ExactArgs(1),
		RunE: runLLMEdit,
	}

	// 添加选项
	llmEditCmd.Flags().StringP("instruction", "i", "", "Editing instruction (required)")
	llmEditCmd.Flags().StringP("target", "t", "", "Target function/class name")
	llmEditCmd.Flags().StringP("context", "c", "", "Context file for reference")
	llmEditCmd.Flags().StringP("language", "L", "", "Specify language (auto-detected by default)")
	llmEditCmd.Flags().BoolP("dry-run", "d", false, "Dry run - show what would be changed")
	llmEditCmd.Flags().BoolP("verbose", "v", false, "Verbose output")

	llmEditCmd.MarkFlagRequired("instruction")

	AddRootCommand(llmEditCmd)
}

// runLLMEdit 是 llm-edit 子命令的入口函数
func runLLMEdit(cmd *cobra.Command, args []string) error {
	filePath := args[0]

	// 检查文件是否存在
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return fmt.Errorf("file not found: %s", filePath)
	}

	// 获取选项
	instruction, _ := cmd.Flags().GetString("instruction")
	target, _ := cmd.Flags().GetString("target")
	contextFile, _ := cmd.Flags().GetString("context")
	language, _ := cmd.Flags().GetString("language")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
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
	editReq := LLMEditRequest{
		FilePath:    filePath,
		Language:    language,
		Instruction: instruction,
		Target:      target,
		Context:     contextContent,
	}

	if verbose {
		fmt.Fprintf(os.Stderr, "File: %s\n", filePath)
		fmt.Fprintf(os.Stderr, "Language: %s\n", language)
		fmt.Fprintf(os.Stderr, "Target: %s\n", target)
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

		fmt.Println("\nWould perform LLM edit with content-based approach")
		return nil
	}

	// 准备LLM请求
	return performLLMContentEdit(cmd.Context(), editReq, string(content), fs, verbose)
}

// performLLMContentEdit 执行基于内容的LLM编辑
func performLLMContentEdit(ctx context.Context, req LLMEditRequest, content string, fs *FileStructure, verbose bool) error {
	// 构建系统提示
	systemPrompt := buildLLMEditSystemPrompt(req, fs)

	// 构建用户消息
	userMessage := buildLLMEditUserMessage(req, content)

	if verbose {
		fmt.Fprintf(os.Stderr, "=== System Prompt ===\n%s\n\n", systemPrompt)
		fmt.Fprintf(os.Stderr, "=== User Message ===\n%s\n\n", userMessage)
	}

	// 调用LLM API
	messages := []Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMessage},
	}

	resp, err := DeepseekClient.Chat(ModelDeepseekChat, messages, nil)
	if err != nil {
		return fmt.Errorf("LLM API error: %w", err)
	}

	// 解析LLM响应
	editResp, err := parseLLMEditResponse(resp.Choices[0].Message.Content)
	if err != nil {
		return fmt.Errorf("failed to parse LLM response: %w", err)
	}

	if !editResp.Success {
		return fmt.Errorf("LLM edit failed: %s", editResp.Error)
	}

	// 显示结果
	fmt.Println("=== LLM Edit Result ===")
	if editResp.Explanation != "" {
		fmt.Printf("Explanation: %s\n\n", editResp.Explanation)
	}

	// 应用修改
	if editResp.OldText != "" && editResp.NewText != "" {
		fmt.Printf("Applying content-based replacement...\n")

		// 使用正则表达式进行替换（支持多行）
		oldTextEscaped := regexp.QuoteMeta(editResp.OldText)
		re := regexp.MustCompile(oldTextEscaped)

		if !re.MatchString(content) {
			return fmt.Errorf("old text not found in file")
		}

		newContent := re.ReplaceAllString(content, editResp.NewText)

		// 写回文件
		if err := os.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}

		fmt.Printf("Successfully updated file: %s\n", req.FilePath)

		// 显示差异
		if verbose {
			fmt.Println("\n=== Changes ===")
			fmt.Println("Old text:")
			fmt.Println(editResp.OldText)
			fmt.Println("\nNew text:")
			fmt.Println(editResp.NewText)
		}
	} else {
		fmt.Println("No specific replacement provided, showing LLM response:")
		fmt.Println(resp.Choices[0].Message.Content)
	}

	return nil
}

// buildLLMEditSystemPrompt 构建LLM编辑系统提示
func buildLLMEditSystemPrompt(req LLMEditRequest, fs *FileStructure) string {
	var sb strings.Builder

	sb.WriteString("You are an expert code editor. Your task is to edit code based on the given instruction.\n\n")
	sb.WriteString("IMPORTANT: You must respond with a JSON object containing the exact text to replace and the new text.\n\n")

	sb.WriteString("File Information:\n")
	sb.WriteString(fmt.Sprintf("- File: %s\n", req.FilePath))
	sb.WriteString(fmt.Sprintf("- Language: %s\n", req.Language))

	if req.Target != "" {
		sb.WriteString(fmt.Sprintf("- Target: %s\n", req.Target))
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
	sb.WriteString("1. Use CONTENT-BASED matching, NOT line numbers\n")
	sb.WriteString("2. Identify the exact text to replace (old_text)\n")
	sb.WriteString("3. Provide the complete new text (new_text)\n")
	sb.WriteString("4. Preserve code style and formatting\n")
	sb.WriteString("5. Maintain backward compatibility unless specified\n")
	sb.WriteString("6. Add appropriate comments for significant changes\n")
	sb.WriteString("7. Follow language-specific best practices\n")

	sb.WriteString("\nOutput Format:\n")
	sb.WriteString("Respond with a JSON object containing:\n")
	sb.WriteString("- success: boolean indicating if edit was successful\n")
	sb.WriteString("- old_text: the exact text to replace (must match exactly in the file)\n")
	sb.WriteString("- new_text: the new text to replace with\n")
	sb.WriteString("- explanation: brief explanation of changes\n")
	sb.WriteString("- error: error message if failed\n")

	sb.WriteString("\nExample:\n")
	sb.WriteString(`{
  "success": true,
  "old_text": "func greet(name string) {\n    fmt.Printf(\"Hello, %s!\", name)\n}",
  "new_text": "func greet(name string) error {\n    if name == \"\" {\n        return fmt.Errorf(\"name cannot be empty\")\n    }\n    fmt.Printf(\"Hello, %s!\", name)\n    return nil\n}",
  "explanation": "Added error handling for empty name"
}`)

	return sb.String()
}

// buildLLMEditUserMessage 构建用户消息
func buildLLMEditUserMessage(req LLMEditRequest, content string) string {
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

// parseLLMEditResponse 解析LLM编辑响应
func parseLLMEditResponse(response string) (*LLMEditResponse, error) {
	// 尝试解析JSON响应
	var editResp LLMEditResponse
	if err := json.Unmarshal([]byte(response), &editResp); err == nil {
		return &editResp, nil
	}

	// 如果不是有效的JSON，尝试提取JSON部分
	lines := strings.Split(response, "\n")
	var jsonLines []string
	var inJSON bool

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "{" || inJSON {
			jsonLines = append(jsonLines, line)
			inJSON = true

			if trimmed == "}" {
				break
			}
		}
	}

	if len(jsonLines) > 0 {
		if err := json.Unmarshal([]byte(strings.Join(jsonLines, "\n")), &editResp); err == nil {
			return &editResp, nil
		}
	}

	// 如果无法解析为JSON，返回错误
	return &LLMEditResponse{
		Success: false,
		Error:   "LLM response is not valid JSON",
	}, nil
}
