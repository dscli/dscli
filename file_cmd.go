package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/dscli/dscli/internal/config"
	"github.com/dscli/dscli/internal/dsc"
	"github.com/nanjj/clog"
	"github.com/spf13/cobra"
)

func init() {
	AddRootCommand(newFileCmd())
}

// newFileCmd 构造 file 命令组（提取为函数便于测试直接构造子命令）。
func newFileCmd() *cobra.Command {
	fileCmd := &cobra.Command{
		Use:   "file",
		Short: "Files API 管理：上传、列出、查询、删除",
		Long: `file 命令用于管理 DeepSeek Files API 中的文件。

本地上传缓存位于 ~/.dscli/files.json（可用配置 files-cache-path 覆盖）：
文件内容（SHA-256 + 大小）相同的重复上传直接复用已上传的 file_id。`,
	}
	fileCmd.AddCommand(newFileUploadCmd())
	fileCmd.AddCommand(newFileListCmd())
	fileCmd.AddCommand(newFileInfoCmd())
	fileCmd.AddCommand(newFileDeleteCmd())
	return fileCmd
}

// newFileUploadCmd 上传子命令：<path> 必选。
func newFileUploadCmd() *cobra.Command {
	uploadCmd := &cobra.Command{
		Use:   "upload <path>",
		Short: "上传本地文件，内容缓存命中时直接复用已上传的 file_id",
		Args:  cobra.ExactArgs(1),
		RunE:  fileUploadRunE,
	}
	uploadCmd.Flags().String("purpose", "", "文件用途（默认 user_data）")
	uploadCmd.Flags().Int64("expires", 0, "有效期秒数（3600-2592000），0 表示永久")
	uploadCmd.Flags().Bool("no-cache", false, "跳过本地缓存，强制重新上传")
	uploadCmd.Flags().StringP("format", "f", "id", "输出格式：id（默认，仅打印 file_id）或 json")
	return uploadCmd
}

// newFileListCmd 列出子命令。
func newFileListCmd() *cobra.Command {
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "列出已上传的文件",
		Args:  cobra.NoArgs,
		RunE:  fileListRunE,
	}
	listCmd.Flags().Int("limit", 20, "返回条数上限")
	listCmd.Flags().String("after", "", "分页游标：仅返回此 file_id 之后的文件")
	listCmd.Flags().String("order", "", "排序：asc 或 desc")
	listCmd.Flags().String("purpose", "", "按用途过滤")
	listCmd.Flags().StringP("format", "f", "table", "输出格式：table（默认）或 json")
	return listCmd
}

// newFileInfoCmd 查询子命令：<file_id> 必选。
func newFileInfoCmd() *cobra.Command {
	infoCmd := &cobra.Command{
		Use:   "info <file_id>",
		Short: "查询单个文件信息",
		Args:  cobra.ExactArgs(1),
		RunE:  fileInfoRunE,
	}
	infoCmd.Flags().StringP("format", "f", "table", "输出格式：table（默认）或 json")
	return infoCmd
}

// newFileDeleteCmd 删除子命令：<file_id> 必选。
func newFileDeleteCmd() *cobra.Command {
	deleteCmd := &cobra.Command{
		Use:   "delete <file_id>",
		Short: "删除文件（同时清理本地缓存）",
		Args:  cobra.ExactArgs(1),
		RunE:  fileDeleteRunE,
	}
	return deleteCmd
}

// newCLIFilesAPI 从配置构造 Files API 客户端（与 chat --attach 同配置；
// 缓存路径由 NewFilesAPI 统一处理 files-cache-path 配置）。
func newCLIFilesAPI() *dsc.FilesAPI {
	key := config.Get("deepseek-api-key", "")
	url := config.Get("deepseek-base-url", "https://api.deepseek.com")
	return dsc.NewFilesAPI(key, url)
}

func fileUploadRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "fileUpload")
	defer span.Finish()

	purpose, _ := cmd.Flags().GetString("purpose")
	expires, _ := cmd.Flags().GetInt64("expires")
	noCache, _ := cmd.Flags().GetBool("no-cache")
	format, _ := cmd.Flags().GetString("format")

	file, err := newCLIFilesAPI().Upload(ctx, args[0], dsc.UploadOptions{
		Purpose:        purpose,
		ExpiresSeconds: expires,
		NoCache:        noCache,
	})
	if err != nil {
		return err
	}
	if format == "json" {
		return FormatOutputToWriter(cmd.OutOrStdout(), file, "json", nil, nil)
	}
	fmt.Fprintln(cmd.OutOrStdout(), file.ID)
	return nil
}

func fileListRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "fileList")
	defer span.Finish()

	limit, _ := cmd.Flags().GetInt("limit")
	after, _ := cmd.Flags().GetString("after")
	order, _ := cmd.Flags().GetString("order")
	purpose, _ := cmd.Flags().GetString("purpose")
	format, _ := cmd.Flags().GetString("format")

	list, err := newCLIFilesAPI().List(ctx, after, limit, order, purpose)
	if err != nil {
		return err
	}
	return FormatOutputToWriter(cmd.OutOrStdout(), list.Data, format, fileHeaders, fileRowFunc)
}

func fileInfoRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "fileInfo")
	defer span.Finish()

	format, _ := cmd.Flags().GetString("format")

	file, err := newCLIFilesAPI().Info(ctx, args[0])
	if err != nil {
		return err
	}
	return FormatOutputToWriter(cmd.OutOrStdout(), []dsc.FileObject{*file}, format, fileHeaders, fileRowFunc)
}

func fileDeleteRunE(cmd *cobra.Command, args []string) error {
	span, ctx := clog.StartSpanFromContext(cmd.Context(), "fileDelete")
	defer span.Finish()

	result, err := newCLIFilesAPI().Delete(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deleted %s\n", result.ID)
	return nil
}

// fileHeaders 是 file 列表/详情表格的列头。
var fileHeaders = []string{"ID", "Bytes", "Created", "Filename", "Purpose", "Expires"}

// fileRowFunc 把 dsc.FileObject 渲染成表格行。
func fileRowFunc(data any) []string {
	f, ok := data.(dsc.FileObject)
	if !ok {
		return nil
	}
	return []string{
		f.ID, strconv.FormatInt(f.Bytes, 10), formatFileTime(f.CreatedAt),
		f.Filename, f.Purpose, formatFileTime(f.ExpiresAt),
	}
}

// formatFileTime 渲染 Unix 时间戳为本地时间；0（未知/永久）显示为空。
func formatFileTime(ts int64) string {
	if ts == 0 {
		return ""
	}
	return time.Unix(ts, 0).Format("2006-01-02 15:04")
}
