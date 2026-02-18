package cmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/nanjunjie/dscli/internal/api"
	"gitcode.com/nanjunjie/dscli/internal/db"
	"github.com/spf13/cobra"
)

var (
	chatModel string
)

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "与 DeepSeek 对话（多轮会话，按项目目录自动隔离）",
	Long: `发送一条消息给 DeepSeek 聊天模型并获取回复。
消息内容必须通过标准输入提供。
对话历史按项目目录自动隔离：每个 Git 仓库（或当前目录）拥有独立的对话上下文。
数据库统一存储在 ~/.dscli/sqlite.db 中。

示例：
  echo "你好" | dscli chat
  echo "继续刚才的话题" | dscli chat
  cat prompt.txt | dscli chat --model deepseek-chat`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. 读取标准输入
		reader := bufio.NewReader(os.Stdin)
		content, err := io.ReadAll(reader)
		if err != nil {
			fmt.Fprintf(os.Stderr, "读取标准输入失败: %v\n", err)
			os.Exit(1)
		}
		userMsg := strings.TrimSpace(string(content))
		if userMsg == "" {
			fmt.Fprintln(os.Stderr, "错误: 标准输入为空，请通过管道或重定向提供消息内容")
			os.Exit(1)
		}

		// 2. 确定项目根路径（用于隔离会话）
		projectPath, err := getProjectRoot()
		if err != nil {
			fmt.Fprintf(os.Stderr, "无法确定项目根路径: %v\n", err)
			os.Exit(1)
		}

		// 3. 打开数据库（统一位置 ~/.dscli/sqlite.db）
		database, err := db.New()
		if err != nil {
			fmt.Fprintf(os.Stderr, "初始化数据库失败: %v\n", err)
			os.Exit(1)
		}
		defer database.Close()

		// 4. 获取会话ID（基于项目路径）
		sessionID, err := database.GetOrCreateSession(projectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "获取会话失败: %v\n", err)
			os.Exit(1)
		}

		// 5. 加载历史消息
		history, err := database.LoadHistory(sessionID)
		if err != nil {
			fmt.Fprintf(os.Stderr, "加载历史消息失败: %v\n", err)
			os.Exit(1)
		}

		// 6. 构造 messages 切片
		messages := make([]api.Message, 0, len(history)+1)
		for _, m := range history {
			messages = append(messages, api.Message{Role: m.Role, Content: m.Content})
		}
		messages = append(messages, api.Message{Role: "user", Content: userMsg})

		// 7. 调用 API
		resp, err := client.Chat(chatModel, messages)
		if err != nil {
			fmt.Fprintf(os.Stderr, "聊天请求失败: %v\n", err)
			os.Exit(1)
		}
		if len(resp.Choices) == 0 {
			fmt.Fprintln(os.Stderr, "错误: 未收到回复")
			os.Exit(1)
		}
		assistantMsg := resp.Choices[0].Message.Content

		// 8. 将本次对话存入数据库
		if err := database.SaveMessages(sessionID, userMsg, assistantMsg); err != nil {
			fmt.Fprintf(os.Stderr, "保存消息失败: %v\n", err)
			os.Exit(1)
		}

		// 9. 输出回复
		fmt.Println(assistantMsg)
	},
}

// getProjectRoot 获取当前项目根目录（用于会话隔离）
// 如果在 Git 仓库内，返回 Git 根目录；否则返回当前目录的绝对路径。
func getProjectRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	// 尝试查找 Git 仓库根目录
	gitRoot, err := findGitRoot(cwd)
	if err == nil && gitRoot != "" {
		return gitRoot, nil
	}

	// 没有 Git 仓库，返回当前目录的绝对路径
	return filepath.Abs(cwd)
}

// findGitRoot 从指定目录向上查找，直到找到包含 .git 的目录
func findGitRoot(dir string) (string, error) {
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}

	for {
		gitPath := filepath.Join(absDir, ".git")
		if _, err := os.Stat(gitPath); err == nil {
			return absDir, nil
		}
		parent := filepath.Dir(absDir)
		if parent == absDir {
			break
		}
		absDir = parent
	}
	return "", fmt.Errorf("未找到 Git 仓库根目录")
}

func init() {
	chatCmd.Flags().StringVar(&chatModel, "model", "deepseek-chat", "使用的模型名称")
	// 注意：已移除 --session 参数
	rootCmd.AddCommand(chatCmd)
}