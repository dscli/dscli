package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/session"
	"github.com/spf13/cobra"
)

func init() {
	projectCmd := AddRootCommand(&cobra.Command{
		Use:   "project",
		Short: "项目管理 - 列出项目",
		Long:  `project 命令用于管理 dscli 追踪的项目。`,
	})

	projectCmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出所有项目",
		Long:  "列出 sessions 表中所有 dscli 追踪的项目，按 ID 排序。",
		Args:  cobra.NoArgs,
		RunE:  projectListRunE,
	})
}

func projectListRunE(_ *cobra.Command, _ []string) error {
	projects, err := session.ListProjects()
	if err != nil {
		return fmt.Errorf("列出项目失败: %w", err)
	}

	if len(projects) == 0 {
		fmt.Println("没有项目。")
		return nil
	}

	type row struct {
		ID         string
		Project    string
		Maintainer string
		CreatedAt  string
	}

	formatTime := func(raw string) string {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			t, err = time.Parse("2006-01-02 15:04:05", raw)
			if err != nil {
				return raw
			}
		}
		return t.Local().Format(time.DateTime)
	}

	var rows []row
	home := os.Getenv("HOME")
	for _, p := range projects {
		projectPath := p.ProjectPath
		if home != "" {
			projectPath = strings.Replace(projectPath, home, "~", 1)
		}
		maintainer := ""
		if p.MaintainerID > 0 {
			maintainer = fmt.Sprintf("%s(%s, %d)", p.MaintainerCN, p.MaintainerEN, p.MaintainerID)
		}
		rows = append(rows, row{
			ID:         strconv.FormatInt(p.ID, 10),
			Project:    projectPath,
			Maintainer: maintainer,
			CreatedAt:  formatTime(p.CreatedAt),
		})
	}

	headers := []string{"ID", "Project", "Maintainer", "Created At"}
	rowFunc := func(data any) []string {
		if r, ok := data.(row); ok {
			return []string{r.ID, r.Project, r.Maintainer, r.CreatedAt}
		}
		return nil
	}

	return FormatOutput(rows, "table", headers, rowFunc)
}
