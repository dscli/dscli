package toolcall

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

// ParseNetrc 解析.netrc文件，支持单行格式：machine host login user password token
func ParseNetrc(path string) ([]NetrcEntry, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var entries []NetrcEntry
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行和注释
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 分割所有字段
		fields := strings.Fields(line)
		if len(fields) < 4 { // 至少需要: machine host login user password token
			continue
		}

		var entry NetrcEntry
		// 解析字段：machine host login user password token
		// 格式：machine <host> login <username> password <token>
		for i := 0; i < len(fields); i++ {
			keyword := strings.ToLower(fields[i])

			switch keyword {
			case "machine":
				if i+1 < len(fields) {
					entry.Machine = fields[i+1]
					i++ // 跳过主机名
				}
			case "login":
				if i+1 < len(fields) {
					entry.Login = fields[i+1]
					i++ // 跳过用户名
				}
			case "password":
				if i+1 < len(fields) {
					entry.Password = fields[i+1]
					i++ // 跳过密码/token
				}
			case "default":
				// 忽略default条目
				return entries, nil
			}
		}

		// 只有包含Machine和Password的条目才有效
		if entry.Machine != "" && entry.Password != "" {
			entries = append(entries, entry)
		}
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
