package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/memories"
	"github.com/spf13/cobra"
)

func init() {
	memCmd := AddRootCommand(&cobra.Command{
		Use:   "memory",
		Short: "记忆管理 - 列出、搜索、查看和删除记忆",
		Long: `memory 命令用于管理持久化记忆。

子命令：
  list    列出当前项目的所有记忆
  search  搜索记忆
  show    查看记忆完整内容
  delete  删除记忆
  stats   显示记忆系统统计`,
	})

	// ── list ────────────────────────────────────────────────────────────
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出当前项目的所有记忆",
		Long:  "按创建时间倒序列出当前项目的所有记忆。",
		Args:  cobra.NoArgs,
		RunE:  memListRunE,
	}
	memCmd.AddCommand(listCmd)

	// ── search ──────────────────────────────────────────────────────────
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "搜索记忆",
		Long: `使用 FTS5 全文搜索记忆。

支持中文分词和英文通配符。`,
		Args: cobra.ExactArgs(1),
		RunE: memSearchRunE,
	}
	searchCmd.Flags().StringP("type", "t", "", "按类型过滤（如 decision, bugfix, pattern）")
	searchCmd.Flags().IntP("limit", "n", 10, "最大结果数")
	memCmd.AddCommand(searchCmd)

	// ── show ────────────────────────────────────────────────────────────
	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "查看记忆完整内容",
		Long:  "根据 ID 查看记忆的完整内容。",
		Args:  cobra.ExactArgs(1),
		RunE:  memShowRunE,
	}
	memCmd.AddCommand(showCmd)

	// ── delete ──────────────────────────────────────────────────────────
	deleteCmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "删除记忆",
		Long:  "根据 ID 删除一条记忆。此操作不可逆。",
		Args:  cobra.ExactArgs(1),
		RunE:  memDeleteRunE,
	}
	memCmd.AddCommand(deleteCmd)

	// ── stats ───────────────────────────────────────────────────────────
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "显示记忆系统统计",
		Long:  "显示当前项目的记忆总数和类型分布。",
		Args:  cobra.NoArgs,
		RunE:  memStatsRunE,
	}
	memCmd.AddCommand(statsCmd)
}

// ─── RunE helpers ──────────────────────────────────────────────────────────

func memListRunE(cmd *cobra.Command, _ []string) error {
	rows, err := memories.HandleMemList(context.Background())
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		fmt.Println("📊 记忆系统为空，还没有任何记忆。")
		return nil
	}

	// firstLine returns the first line of content (up to \n).
	firstLine := func(s string) string {
		if idx := strings.IndexByte(s, '\n'); idx >= 0 {
			return s[:idx]
		}
		return s
	}

	// truncateRunes truncates s to max runes, appending "..." if needed.
	truncateRunes := func(s string, max int) string {
		runes := []rune(s)
		if len(runes) <= max {
			return s
		}
		return string(runes[:max]) + "..."
	}

	// formatTime parses SQLite datetime (RFC3339 from modernc driver)
	// and formats as Go time.Stamp ("Jan _2 15:04:05").
	formatTime := func(raw string) string {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			// Fallback: SQLite space-separated format
			t, err = time.Parse("2006-01-02 15:04:05", raw)
			if err != nil {
				return raw
			}
		}
		return t.Format(time.Stamp)
	}

	type row struct {
		ID        string
		Title     string
		Content   string
		CreatedAt string
	}

	var data []row
	for _, r := range rows {
		content := truncateRunes(firstLine(r.Content), 20)
		data = append(data, row{
			ID:        strconv.FormatInt(r.ID, 10),
			Title:     r.Title,
			Content:   content,
			CreatedAt: formatTime(r.CreatedAt),
		})
	}

	headers := []string{"ID", "TITLE", "CONTENT", "Created At"}
	rowFn := func(d any) []string {
		if r, ok := d.(row); ok {
			return []string{r.ID, r.Title, r.Content, r.CreatedAt}
		}
		return nil
	}

	return FormatOutput(data, "table", headers, rowFn)
}

func memSearchRunE(cmd *cobra.Command, args []string) error {
	query := args[0]
	typ, _ := cmd.Flags().GetString("type")
	limit, _ := cmd.Flags().GetInt("limit")

	result, warning, err := memories.HandleMemSearch(context.Background(), query, typ, limit)
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️ ", warning)
	}
	fmt.Print(result)
	return nil
}

func memShowRunE(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的 ID: %w", err)
	}

	result, warning, err := memories.HandleMemGetObservation(context.Background(), id)
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️ ", warning)
	}
	fmt.Println(result)
	return nil
}

func memDeleteRunE(cmd *cobra.Command, args []string) error {
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return fmt.Errorf("无效的 ID: %w", err)
	}

	result, warning, err := memories.HandleMemDelete(context.Background(), id)
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️ ", warning)
	}
	// HandleMemDelete already prints via outfmt.Printf, result is for LLM
	fmt.Println(result)
	return nil
}

func memStatsRunE(cmd *cobra.Command, _ []string) error {
	result, warning, err := memories.HandleMemStats(context.Background())
	if err != nil {
		return err
	}
	if warning != "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "⚠️ ", warning)
	}
	fmt.Print(result)
	return nil
}
