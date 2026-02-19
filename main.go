package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/joho/godotenv"
	"gitcode.com/nanjunjie/dscli/cmd"
)

func init() {
	// 可能的配置文件路径（按优先级排序）
	configPaths := []string{
		// 1. 用户主目录的 .dscli/.env 文件（标准格式，最高优先级）
		filepath.Join(os.Getenv("HOME"), ".dscli", ".env"),
		// 2. 用户主目录的 .dscli/dscli.env 文件（兼容旧格式）
		filepath.Join(os.Getenv("HOME"), ".dscli", "dscli.env"),
		// 3. 当前目录的 .env 文件
		".env",
	}

	loaded := false
	// 尝试加载配置文件
	for _, path := range configPaths {
		if loadConfig(path) {
			loaded = true
			break // 成功加载一个配置文件就停止
		}
	}

	// 如果没有找到配置文件，输出提示信息
	if !loaded {
		fmt.Fprintf(os.Stderr, "ℹ️  提示: 未找到配置文件，请设置 DEEPSEEK_API_KEY 环境变量\n")
		fmt.Fprintf(os.Stderr, "     或在以下位置创建配置文件:\n")
		for _, path := range configPaths {
			fmt.Fprintf(os.Stderr, "     - %s\n", path)
		}
		fmt.Fprintf(os.Stderr, "\n配置文件格式示例:\n")
		fmt.Fprintf(os.Stderr, "DEEPSEEK_API_KEY=your_api_key_here\n")
		fmt.Fprintf(os.Stderr, "DEEPSEEK_BASE_URL=https://api.deepseek.com/beta\n")
		fmt.Fprintf(os.Stderr, "\n")
	}
}

// loadConfig 尝试加载配置文件
func loadConfig(path string) bool {
	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}

	// 判断文件格式
	lines := strings.Split(string(content), "\n")
	isShellFormat := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "export ") {
			isShellFormat = true
			break
		}
	}

	if isShellFormat {
		// Shell格式：需要解析 export KEY=VALUE
		return loadShellFormatConfig(content, path)
	} else {
		// 标准 .env 格式：直接使用 godotenv
		if err := godotenv.Load(path); err != nil {
			fmt.Fprintf(os.Stderr, "⚠️  警告: 加载配置文件 %s 失败: %v\n", path, err)
			return false
		}
		fmt.Fprintf(os.Stderr, "✅ 已加载配置文件: %s\n", path)
		return true
	}
}

// loadShellFormatConfig 加载shell格式的配置文件
func loadShellFormatConfig(content []byte, path string) bool {
	lines := strings.Split(string(content), "\n")
	envVars := make(map[string]string)

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 移除 export 前缀
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimPrefix(line, "export ")
		}

		// 解析 KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 移除引号
		value = strings.Trim(value, `"'`)

		envVars[key] = value
	}

	// 设置环境变量
	for key, value := range envVars {
		if os.Getenv(key) == "" {
			os.Setenv(key, value)
		}
	}

	fmt.Fprintf(os.Stderr, "✅ 已加载配置文件: %s\n", path)
	return true
}

func main() {
	cmd.Execute()
}
