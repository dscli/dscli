package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// NetrcEntry 表示.netrc文件中的一个条目
type NetrcEntry struct {
	Machine  string
	Login    string
	Password string
}

// ParseNetrc 解析.netrc文件，返回所有条目
func ParseNetrc(path string) ([]NetrcEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []NetrcEntry
	var current *NetrcEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) < 2 {
			// 无效行，重置current
			current = nil
			continue
		}

		keyword := fields[0]
		value := fields[1]

		switch keyword {
		case "machine":
			// 保存前一个完整的条目
			if current != nil && current.Machine != "" {
				entries = append(entries, *current)
			}
			// 开始新条目
			current = &NetrcEntry{Machine: value}
		case "login":
			if current != nil {
				current.Login = value
			}
		case "password":
			if current != nil {
				current.Password = value
			}
		case "default":
			// 遇到default，保存前一个完整的条目
			if current != nil && current.Machine != "" {
				entries = append(entries, *current)
			}
			// 完全忽略default条目
			current = nil
		default:
			// 未知关键字，重置current
			current = nil
		}
	}

	// 添加最后一个完整的条目
	if current != nil && current.Machine != "" {
		entries = append(entries, *current)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// GetTokenFromNetrc 从.netrc文件获取指定主机的token
func GetTokenFromNetrc(host string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	netrcPath := filepath.Join(home, ".netrc")
	entries, err := ParseNetrc(netrcPath)
	if err != nil {
		// 文件不存在或无法读取，返回空但不报错
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	// 查找匹配的条目
	for _, entry := range entries {
		if entry.Machine == host && entry.Password != "" {
			return entry.Password, nil
		}
	}

	// 没有找到匹配的条目
	return "", nil
}
