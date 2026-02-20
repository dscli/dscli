package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/joho/godotenv"
	"gitcode.com/nanjunjie/dscli/cmd"
)

func init() {
	// 可能的配置文件路径（按优先级排序）
	configPaths := []string{
		// 1. 用户主目录的 .dscli/.env 文件（标准格式，最高优先级）
		filepath.Join(os.Getenv("HOME"), ".dscli", ".env"),
		// 2. 用户主目录的 .dscli/dscli.env 文件（兼容旧格式，但只支持标准.env格式）
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

	// 使用 godotenv 加载标准 .env 格式文件
	if err := godotenv.Load(path); err != nil {
		fmt.Fprintf(os.Stderr, "⚠️  警告: 加载配置文件 %s 失败: %v\n", path, err)
		return false
	}
	
	fmt.Fprintf(os.Stderr, "✅ 已加载配置文件: %s\n", path)
	return true
}

func main() {
	// 确保程序退出时关闭日志
	cmd.Execute()
}
