package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// expandFilePattern 扩展文件模式
func expandFilePattern(pattern string) ([]string, error) {
	// 1. 当前目录：.
	if pattern == "." {
		return listNonHiddenFiles(".")
	}

	// 2. 递归搜索：**/*
	if strings.HasPrefix(pattern, "**/") {
		return recursiveGlob(pattern[3:])
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
func recursiveGlob(pattern string) ([]string, error) {
	var files []string

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // 跳过错误文件
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
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				return nil // 跳过模式错误
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
