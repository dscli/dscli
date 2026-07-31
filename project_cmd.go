package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/session"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"
)

func init() {
	projectCmd := AddRootCommand(&cobra.Command{
		Use:   "project",
		Short: "项目管理 - 列出项目",
		Long:  `project 命令用于管理 dscli 追踪的项目。`,
	})

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出所有项目",
		Long:  "列出 sessions 表中所有 dscli 追踪的项目，按 ID 排序。",
		Args:  cobra.NoArgs,
		RunE:  projectListRunE,
	}
	listCmd.Flags().Bool("json", false, "Output in JSON format (raw fields, machine-readable)")
	projectCmd.AddCommand(listCmd)

	projectCmd.AddCommand(&cobra.Command{
		Use:   "assign <project_id> <maintainer_id>",
		Short: "指定项目的维护者",
		Long: `将指定项目（session）指派给一个 AI 维护者。

示例:
  dscli project assign 7 30    # 将项目 7 指派给张衡(id=30)`,
		Args: cobra.ExactArgs(2),
		RunE: projectAssignRunE,
	})

	projectCmd.AddCommand(&cobra.Command{
		Use:   "update <project_id> <project>",
		Short: "更新项目的路径",
		Long: `更新指定项目（session）的 project_path。

示例:
  dscli project update 2 /new/path/to/project`,
		Args: cobra.ExactArgs(2),
		RunE: projectUpdateRunE,
	})

	projectCmd.AddCommand(&cobra.Command{
		Use:   "remove <project_id|project_path>",
		Short: "删除指定项目",
		Long: `从数据库中删除指定项目（session）及其所有关联数据。

支持按项目 ID 或项目路径删除。

示例:
  dscli project remove 6          # 删除项目 6
  dscli project remove /home/user/tmp  # 按路径删除`,
		Args: cobra.ExactArgs(1),
		RunE: projectRemoveRunE,
	})
}

func projectListRunE(cmd *cobra.Command, _ []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "projectListRunE")
	defer span.Finish()

	// 确保当前项目已分配 session，这样即使首次访问也能列出来并标记箭头。
	session.GetCurrentSessionID(ctx)

	projects, err := session.ListProjects(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "列出项目失败: %v\n", err)
		return nil
	}

	useJSON, _ := cmd.Flags().GetBool("json")
	if useJSON {
		return projectListJSON(projects)
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
	currentRoot := context.ProjectRoot
	for _, p := range projects {
		projectPath := p.ProjectPath
		if home != "" {
			projectPath = strings.Replace(projectPath, home, "~", 1)
		}
		maintainer := ""
		if p.MaintainerID > 0 {
			maintainer = fmt.Sprintf("%s(%s, %d)", p.MaintainerCN, p.MaintainerEN, p.MaintainerID)
		}
		idStr := strconv.FormatInt(p.ID, 10)
		if p.ProjectPath == currentRoot {
			idStr = idStr + " →"
		}
		rows = append(rows, row{
			ID:         idStr,
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

// projectListJSON 以 JSON 格式输出项目列表。
//
// 字段与数据库同源（session.ProjectRow），不做表格展示层的转换
// （~ 替换、→ 标记、维护者合并字符串），方便扩展等客户端直接消费。
// created_at 统一转为 RFC3339，与 history list --json 保持一致。
func projectListJSON(projects []session.ProjectRow) error {
	type projectEntry struct {
		ID           int64  `json:"id"`
		ProjectPath  string `json:"project_path"`
		MaintainerCN string `json:"maintainer_cn"`
		MaintainerEN string `json:"maintainer_en"`
		MaintainerID int64  `json:"maintainer_id"`
		CreatedAt    string `json:"created_at"`
		IsCurrent    bool   `json:"is_current"`
	}
	result := make([]projectEntry, 0, len(projects))
	for _, p := range projects {
		result = append(result, projectEntry{
			ID:           p.ID,
			ProjectPath:  p.ProjectPath,
			MaintainerCN: p.MaintainerCN,
			MaintainerEN: p.MaintainerEN,
			MaintainerID: p.MaintainerID,
			CreatedAt:    formatProjectTimeRFC3339(p.CreatedAt),
			IsCurrent:    p.ProjectPath == context.ProjectRoot,
		})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// formatProjectTimeRFC3339 将数据库中的 created_at 字符串转为 RFC3339。
// 数据库可能存 RFC3339 或本地时间 "2006-01-02 15:04:05"（无时区，按本地时区解释），
// 解析失败时原样返回，保证不丢数据。
func formatProjectTimeRFC3339(raw string) string {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.Format(time.RFC3339)
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local); err == nil {
		return t.Format(time.RFC3339)
	}
	return raw
}

// projectAssignRunE handles "dscli project assign <project_id> <maintainer_id>".
func projectAssignRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "projectAssignRunE")
	defer span.Finish()

	projectID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || projectID <= 0 {
		return fmt.Errorf("无效的 project_id: %s（需要正整数）", args[0])
	}
	maintainerID, err := strconv.ParseInt(args[1], 10, 64)
	if err != nil || maintainerID <= 0 {
		return fmt.Errorf("无效的 maintainer_id: %s（需要正整数）", args[1])
	}

	if err := session.AssignMaintainer(ctx, projectID, maintainerID); err != nil {
		fmt.Fprintf(os.Stderr, "指派维护者失败: %v\n", err)
		return nil
	}

	fmt.Printf("已将项目 %d 指派给 maintainer %d。\n", projectID, maintainerID)
	return nil
}

// projectUpdateRunE handles "dscli project update <project_id> <project>".
func projectUpdateRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "projectUpdateRunE")
	defer span.Finish()
	projectID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || projectID <= 0 {
		return fmt.Errorf("无效的 project_id: %s（需要正整数）", args[0])
	}
	newPath := args[1]
	if newPath == "" {
		return fmt.Errorf("project path 不能为空")
	}

	if err := session.UpdateProjectPath(ctx, projectID, newPath); err != nil {
		fmt.Fprintf(os.Stderr, "更新项目路径失败: %v\n", err)
		return nil
	}

	fmt.Printf("已将项目 %d 的路径更新为 %s。\n", projectID, newPath)
	return nil
}

// projectRemoveRunE handles "dscli project remove <project_id|project_path>".
func projectRemoveRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "projectRemoveRunE")
	defer span.Finish()

	arg := args[0]

	// Try as numeric ID first.
	projectID, err := strconv.ParseInt(arg, 10, 64)
	if err != nil || projectID <= 0 {
		// Not a valid ID — try as project path.
		// Resolve ~ to home directory.
		resolvedPath := arg
		if strings.HasPrefix(resolvedPath, "~") {
			home, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("无法解析 ~: %w", err)
			}
			resolvedPath = filepath.Join(home, strings.TrimPrefix(resolvedPath, "~"))
		}

		projects, err := session.ListProjects(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "查询项目列表失败: %v\n", err)
			return nil
		}
		found := false
		for _, p := range projects {
			if p.ProjectPath == resolvedPath {
				projectID = p.ID
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("未找到路径为 %q 的项目", arg)
		}
	}

	if err := session.RemoveProject(ctx, projectID); err != nil {
		fmt.Fprintf(os.Stderr, "删除项目失败: %v\n", err)
		return nil
	}

	fmt.Printf("已删除项目 %d。\n", projectID)
	return nil
}
