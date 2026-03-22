package file

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gitcode.com/dscli/dscli/internal/outfmt"
)

// SearchConfig 搜索配置
type SearchConfig struct {
	MaxDepth        int      // 最大递归深度
	ExcludeDirs     []string // 排除目录列表
	ExcludePatterns []string // 排除模式列表
}

// defaultSearchConfig 默认搜索配置
var defaultSearchConfig = SearchConfig{
	MaxDepth:        10, // 默认最大深度10层
	ExcludeDirs:     []string{".git", "vendor", "node_modules", "__pycache__", ".idea", ".vscode"},
	ExcludePatterns: []string{"*.pyc", "*.class", "*.o", "*.so", "*.dll"},
}

// ExpandFilePattern 扩展文件模式
func ExpandFilePattern(pattern string) ([]string, error) {
	return expandFilePatternWithConfig(pattern, defaultSearchConfig)
}

// expandFilePatternWithConfig 使用配置扩展文件模式
func expandFilePatternWithConfig(pattern string, config SearchConfig) ([]string, error) {
	// 1. 当前目录：.
	if pattern == "." {
		return listNonHiddenFiles(".")
	}

	// 2. 递归搜索：**/*
	if strings.HasPrefix(pattern, "**/") {
		return recursiveGlob(pattern[3:], config)
	}

	// 3. 通配符：*.go
	if strings.Contains(pattern, "*") {
		return filepath.Glob(pattern)
	}

	// 4. 多个文件：main.go root.go
	if strings.Contains(pattern, " ") {
		files := strings.Fields(pattern)
		var validFiles []string
		for _, file := range files {
			if _, err := os.Stat(file); err == nil {
				validFiles = append(validFiles, file)
			}
		}
		return validFiles, nil
	}

	// 5. 单个文件
	if _, err := os.Stat(pattern); err == nil {
		return []string{pattern}, nil
	}

	return nil, fmt.Errorf("未找到匹配的文件: %s", pattern)
}

// listNonHiddenFiles 列出当前目录所有非隐藏文件
func listNonHiddenFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("读取目录失败: %w", err)
	}

	var files []string
	for _, entry := range entries {
		// 跳过隐藏文件和目录
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if entry.IsDir() {
			continue
		}
		files = append(files, entry.Name())
	}

	return files, nil
}

// recursiveGlob 递归搜索文件
func recursiveGlob(pattern string, config SearchConfig) ([]string, error) {
	var files []string
	baseDir := "."

	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// 记录错误但继续
			outfmt.Printf("⚠️ 访问路径 %s 时出错: %v", path, err)
			return nil
		}

		// 计算深度
		relPath, _ := filepath.Rel(baseDir, path)
		depth := 0
		if relPath != "." {
			depth = strings.Count(relPath, string(filepath.Separator)) + 1
		}

		// 检查深度限制
		if config.MaxDepth > 0 && depth > config.MaxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 检查排除目录
		if info.IsDir() {
			for _, excludeDir := range config.ExcludeDirs {
				if info.Name() == excludeDir {
					return filepath.SkipDir
				}
			}
		}

		// 跳过隐藏文件和目录
		if strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// 只匹配文件
		if !info.IsDir() {
			// 检查排除模式
			for _, excludePattern := range config.ExcludePatterns {
				matched, _ := filepath.Match(excludePattern, info.Name())
				if matched {
					return nil
				}
			}

			// 匹配目标模式
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				outfmt.Printf("⚠️ 模式匹配错误 %s: %v", pattern, err)
				return nil
			}
			if matched {
				files = append(files, path)
			}
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("递归搜索失败: %w", err)
	}

	return files, nil
}
