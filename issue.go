package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

type Issue struct {
	ID     int    `json:"id"`
	Number string `json:"number"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

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
			continue
		}

		keyword := fields[0]
		value := fields[1]

		switch keyword {
		case "machine":
			// 保存前一个条目（如果有）
			if current != nil {
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
			// default条目，我们暂时不处理
			if current != nil {
				entries = append(entries, *current)
			}
			current = &NetrcEntry{Machine: "default"}
		}
	}

	// 添加最后一个条目
	if current != nil {
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

func init() {
	issueCmd := &cobra.Command{
		Use: "issue",
	}

	rootCmd.AddCommand(issueCmd)
	var state string
	listCmd := &cobra.Command{
		Use: "list",
		PreRunE: func(cmd *cobra.Command, args []string) (err error) {
			switch state {
			case "open", "closed", "all":
				return
			}
			err = fmt.Errorf("state:%s should be in open, closed or all", state)
			return
		},
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			baseURL, token, err := IssueAPIBaseURL()
			if err != nil {
				return err
			}
			url := fmt.Sprintf("%s?access_token=%s&state=%s", baseURL, token, state)
			resp, err := http.Get(url)
			if err != nil {
				return err
			}
			defer resp.Body.Close()
			b, err := io.ReadAll(resp.Body)
			if err != nil {
				return err
			}
			issues := []Issue{}
			err = json.Unmarshal(b, &issues)
			if err != nil {
				return err
			}
			for _, issue := range issues {
				fmt.Printf("# %s\n", issue.Title)
				fmt.Printf("- id: %d\n", issue.ID)
				fmt.Printf("- number: %s\n", issue.Number)
				fmt.Printf("- state: %s\n", issue.State)
				fmt.Printf("\n%s\n\n", issue.Body)
			}
			return nil
		},
	}
	listCmd.Flags().StringVar(&state, "state", "open", "issue state in open, closed and all, default open")
	showCmd := &cobra.Command{
		Use: "show",
		RunE: func(cmd *cobra.Command, args []string) (err error) {
			return nil
		},
	}

	createCmd := &cobra.Command{
		Use:  "create",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	updateCmd := &cobra.Command{
		Use:  "update",
		RunE: func(cmd *cobra.Command, args []string) error { return nil },
	}

	issueCmd.AddCommand(listCmd, showCmd, updateCmd, createCmd)
}

func IssueAPIBaseURL() (baseURL string, token string, err error) {
	originURL, err := ShellExec(`git remote get-url origin`)
	if err != nil {
		return
	}
	originURL = strings.TrimSpace(originURL)

	// 移除.git后缀
	originURL = strings.TrimSuffix(originURL, ".git")

	// 解析URL，支持SSH和HTTPS格式
	var host, owner, repo string

	if strings.HasPrefix(originURL, "git@") {
		// SSH格式: git@gitcode.com:dscli/dscli
		parts := strings.Split(originURL, ":")
		if len(parts) != 2 {
			err = fmt.Errorf("invalid SSH URL format: %s", originURL)
			return
		}
		host = strings.TrimPrefix(parts[0], "git@")
		path := parts[1]
		pathParts := strings.Split(path, "/")
		if len(pathParts) != 2 {
			err = fmt.Errorf("invalid path in SSH URL: %s", path)
			return
		}
		owner, repo = pathParts[0], pathParts[1]
	} else if strings.HasPrefix(originURL, "http") {
		// HTTPS格式: https://gitcode.com/dscli/dscli
		// 移除协议前缀
		urlWithoutProtocol := strings.TrimPrefix(originURL, "https://")
		urlWithoutProtocol = strings.TrimPrefix(urlWithoutProtocol, "http://")

		parts := strings.Split(urlWithoutProtocol, "/")
		if len(parts) < 3 {
			err = fmt.Errorf("invalid HTTPS URL format: %s", originURL)
			return
		}
		host = parts[0]
		owner, repo = parts[1], parts[2]
	} else {
		err = fmt.Errorf("unsupported URL format: %s", originURL)
		return
	}

	apiHost := map[string]string{
		"gitcode.com": "api.gitcode.com/api/v5",
	}[host]

	if apiHost == "" {
		err = fmt.Errorf("%s not support yet", host)
		return
	}

	// 使用纯Go实现从.netrc获取token
	token, err = GetTokenFromNetrc(host)
	if err != nil {
		return
	}
	if token == "" {
		err = fmt.Errorf("no token found for %s in ~/.netrc", host)
		return
	}

	baseURL = fmt.Sprintf("https://%s/repos/%s/%s/issues",
		apiHost, owner, repo)
	return
}
