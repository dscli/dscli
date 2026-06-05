package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/dscli/dscli/internal/memories"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/spf13/cobra"
)

func init() {
	memCmd := AddRootCommand(&cobra.Command{
		Use:   "memory",
		Short: "记忆管理 - 列出、搜索、查看和统计记忆",
		Long:  `memory 命令用于管理持久化记忆。`,
	})

	// == list ============================================================
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出当前项目的所有记忆",
		Long:  "按创建时间倒序列出当前项目的所有记忆。",
		Args:  cobra.NoArgs,
		RunE:  memListRunE,
	}
	memCmd.AddCommand(listCmd)

	// == search ==========================================================
	searchCmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search memories",
		Long: `使用 FTS5 全文搜索记忆。

支持中文分词和英文通配符。`,
		Args: cobra.ExactArgs(1),
		RunE: memSearchRunE,
	}
	searchCmd.Flags().String("type", "", "按类型过滤（如 decision, bugfix, pattern）")
	searchCmd.Flags().Int("limit", 10, "最大结果数")
	memCmd.AddCommand(searchCmd)

	// == show ============================================================
	showCmd := &cobra.Command{
		Use:   "show <id>",
		Short: "查看记忆完整内容",
		Long:  "根据 ID 查看记忆的完整内容。",
		Args:  cobra.ExactArgs(1),
		RunE:  memShowRunE,
	}
	memCmd.AddCommand(showCmd)

	// == stats ===========================================================
	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "显示记忆系统统计",
		Long:  "显示当前项目的记忆总数和类型分布。",
		Args:  cobra.NoArgs,
		RunE:  memStatsRunE,
	}
	memCmd.AddCommand(statsCmd)
}

// === RunE helpers ==========================================================

func memListRunE(_ *cobra.Command, _ []string) error {
	rows, err := memories.HandleMemList(context.Background())
	if err != nil {
		return err
	}

	if len(rows) == 0 {
		outfmt.Println("📊 记忆系统为空，还没有任何记忆。")
		return nil
	}

	// formatTime parses SQLite datetime (RFC3339 from modernc driver),
	// converts to local timezone, and formats as Go time.Stamp ("Jan _2 15:04:05").
	formatTime := func(raw string) string {
		t, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			// Fallback: SQLite space-separated format
			t, err = time.Parse("2006-01-02 15:04:05", raw)
			if err != nil {
				return raw
			}
		}
		return t.Local().Format(time.Stamp)
	}

	wrt := outfmt.NewTabwrt()
	defer wrt.Flush()

	wrt.Println("ID", "TITLE", "Created At", "Updated At")

	for _, r := range rows {
		wrt.Println(
			strconv.FormatInt(r.ID, 10),
			r.Title,
			formatTime(r.CreatedAt),
			formatTime(r.UpdatedAt),
		)
	}

	return nil
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
	outfmt.Print(result)
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
	outfmt.Println(result)
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
	outfmt.Print(result)
	return nil
}
