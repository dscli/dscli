package file

import (
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/toolcall"
)

const (
	noMatchesFound = "No matches found"
	noFilesFound   = "No files found"
)

type rgJSON struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		LineNumber int `json:"line_number"`
		Lines      struct {
			Text string `json:"text"`
		} `json:"lines"`
	} `json:"data"`
}

// GrepMatch represents a single pattern match result.
type GrepMatch struct {
	Content string `json:"content,omitzero"`

	// Path is the file path where the match was found.
	Path string `json:"path,omitzero"`

	// Line is the 1-based line number of the match.
	Line int `json:"line,omitzero"`
}

func JSONString(matches []GrepMatch) string {
	b, err := json.MarshalIndent(matches, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

func RipgrepExists() bool {
	rg, err := exec.LookPath("rg")
	if err == nil && rg != "" {
		return true
	}
	return false
}

func Ripgrep(ctx context.Context, pattern string, path string, glob string, fileType string, afterLines int64, beforeLines int64, caseInsensitive bool, enableMultiline bool) (matches []GrepMatch, err error) {
	cmd := []string{"rg", "--json"}
	if caseInsensitive {
		cmd = append(cmd, "-i")
	}

	if enableMultiline {
		cmd = append(cmd, "-U", "--multiline-dotall")
	}

	if fileType != "" {
		cmd = append(cmd, "--type", fileType)
	} else if glob != "" {
		cmd = append(cmd, "--glob", glob)
	}

	if afterLines > 0 {
		cmd = append(cmd, "-A", fmt.Sprintf("%d", afterLines))
	}
	if beforeLines > 0 {
		cmd = append(cmd, "-B", fmt.Sprintf("%d", beforeLines))
	}

	cmd = append(cmd, "-e", pattern, "--", path)

	execCmd := exec.CommandContext(ctx, cmd[0], cmd[1:]...)
	output, err := execCmd.Output()
	matches = []GrepMatch{}
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			err = fmt.Errorf("ripgrep (rg) is not installed or not in PATH. Please install it: https://github.com/BurntSushi/ripgrep#installation")
			return
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if exitErr.ExitCode() == 1 {
				return
			}
			err = fmt.Errorf("ripgrep failed with exit code %d: %s", exitErr.ExitCode(), string(exitErr.Stderr))
			return
		}
		err = fmt.Errorf("failed to execute ripgrep: %w", err)
		return
	}

	if len(output) == 0 {
		return
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	var data rgJSON
	for _, line := range lines {
		data = rgJSON{}
		if err := json.Unmarshal([]byte(line), &data); err != nil {
			continue
		}
		if data.Type == "match" || data.Type == "context" {
			matchPath := data.Data.Path.Text
			matches = append(matches, GrepMatch{
				Path:    matchPath,
				Line:    data.Data.LineNumber,
				Content: strings.TrimRight(data.Data.Lines.Text, "\n"),
			})
		}
	}
	return
}

func init() {
	if !RipgrepExists() {
		return
	}
	toolcall.RegisterTool(toolcall.ToolDef{
		Name: "ripgrep",
		Description: `基于 ripgrep 的强大搜索工具

  使用方法：
  - 始终使用 Grep 进行搜索任务。不要将 'grep' 或 'rg' 作为 Bash 命令调用。Grep 工具已针对正确的权限和访问进行了优化
  - 支持完整的正则表达式语法（例如，"log.*Error"，"function\s+\w+"）
  - 使用 glob 参数（例如，"*.js"，"**/*.tsx"）或 type 参数（例如，"js"，"py"，"rust"）过滤文件
  - 输出模式："content" 显示匹配行，"files_with_matches" 仅显示文件路径（默认），"count" 显示匹配计数
  - 对于需要多轮的开放式搜索，使用 Task 工具
  - 模式语法：使用 ripgrep（不是 grep）- 字面大括号需要转义（使用 'interface\{\}' 在 Go 代码中查找 'interface{}'）
  - 多行匹配：默认情况下，模式仅在单行内匹配。对于跨行模式如 'struct \{[\s\S]*?field'，使用 'multiline: true'`,
		Strict: true,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"pattern": map[string]any{
					"type":        "string",
					"description": "The regular expression pattern to search for in file contents",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "File or directory to search in (rg PATH). Defaults to current working directory.",
				},
				"glob": map[string]any{
					"type":        "string",
					"description": "Glob pattern to filter files (e.g. '*.js', '*.{ts,tsx}') - maps to rg --glob",
				},
				"type": map[string]any{
					"type":        "string",
					"description": "File type to search (rg --type). Common types: js, py, rust, go, java, etc. More efficient than include for standard file types.",
				},
				"-i": map[string]any{
					"type":        "boolean",
					"description": "Case insensitive search (rg -i)",
				},
				"-n": map[string]any{
					"type":        "boolean",
					"description": "Show line numbers in output (rg -n). Requires output_mode: 'content', ignored otherwise. Defaults to true.",
				},
				"-A": map[string]any{
					"type":        "integer",
					"description": "Number of lines to show after each match (rg -A). Requires output_mode: 'content', ignored otherwise.",
				},
				"-B": map[string]any{
					"type":        "integer",
					"description": "Number of lines to show before each match (rg -B). Requires output_mode: 'content', ignored otherwise.",
				},
				"-C": map[string]any{
					"type":        "integer",
					"description": "Number of lines to show before and after each match (rg -C). Requires output_mode: 'content', ignored otherwise.",
				},
				"output_mode": map[string]any{
					"type":        "string",
					"description": "Output mode: 'content' shows matching lines (supports -A/-B/-C context, -n line numbers, head_limit), 'files_with_matches' shows file paths (supports head_limit), 'count' shows match counts (supports head_limit). Defaults to 'files_with_matches'.",
					"enum":        []string{"content", "files_with_matches", "count"},
				},
				"head_limit": map[string]any{
					"type":        "integer",
					"description": "Limit output to first N lines/entries, equivalent to '| head -N'. Works across all output modes: content (limits output lines), files_with_matches (limits file paths), count (limits count entries). Defaults to 0 (unlimited).",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Skip first N lines/entries before applying head_limit, equivalent to '| tail -n +N | head -N'. Works across all output modes. Defaults to 0.",
				},
				"multiline": map[string]any{
					"type":        "boolean",
					"description": "Enable multiline mode where . matches newlines and patterns can span lines (rg -U --multiline-dotall). Default: false.",
				},
			},
			"required":             []string{"pattern"},
			"additionalProperties": false,
		},
		Category: "file_ops",
		Handler:  handleRipgrep,
	})
}

func applyPagination[T any](items []T, offset, limit int) []T {
	if offset < 0 {
		offset = 0
	}
	if offset >= len(items) {
		return []T{}
	}
	if limit <= 0 {
		limit = len(items)
	}
	end := min(offset + limit, len(items))
	return items[offset:end]
}

func formatContentMatches(matches []GrepMatch, showLineNumbers bool) string {
	if len(matches) == 0 {
		return noMatchesFound
	}

	var b strings.Builder
	for _, match := range matches {
		if showLineNumbers {
			fmt.Fprintf(&b, "%s:%d:%s\n", match.Path, match.Line, match.Content)
		} else {
			fmt.Fprintf(&b, "%s:%s\n", match.Path, match.Content)
		}
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func formatFileMatches(matches []GrepMatch, offset, headLimit int) string {
	if len(matches) == 0 {
		return noFilesFound
	}

	// 去重文件路径
	fileSet := make(map[string]bool)
	for _, match := range matches {
		fileSet[match.Path] = true
	}

	var files []string
	for file := range fileSet {
		files = append(files, file)
	}
	sort.Strings(files)

	files = applyPagination(files, offset, headLimit)

	var b strings.Builder
	for _, file := range files {
		b.WriteString(file + "\n")
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func formatCountMatches(matches []GrepMatch, offset, headLimit int) string {
	if len(matches) == 0 {
		return noMatchesFound
	}

	// 统计每个文件的匹配次数
	countMap := make(map[string]int)
	for _, match := range matches {
		countMap[match.Path]++
	}

	var paths []string
	for path := range countMap {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	totalOccurrences := len(matches)
	totalFiles := len(paths)

	occurrenceWord := "occurrences"
	if totalOccurrences == 1 {
		occurrenceWord = "occurrence"
	}
	fileWord := "files"
	if totalFiles == 1 {
		fileWord = "file"
	}

	if totalOccurrences == 0 {
		return fmt.Sprintf("%s\n\nFound %d total %s across %d %s.", noMatchesFound, totalOccurrences, occurrenceWord, totalFiles, fileWord)
	}

	paths = applyPagination(paths, offset, headLimit)

	var b strings.Builder
	for _, path := range paths {
		b.WriteString(path)
		b.WriteString(":")
		b.WriteString(strconv.Itoa(countMap[path]))
		b.WriteString("\n")
	}
	result := strings.TrimSuffix(b.String(), "\n")
	return fmt.Sprintf("%s\n\nFound %d total %s across %d %s.", result, totalOccurrences, occurrenceWord, totalFiles, fileWord)
}

func handleRipgrep(ctx context.Context, toolArgs toolcall.ToolArgs) (result string, suggestion string, err error) {
	pattern := toolcall.ToolArgsValue(toolArgs, "pattern", "")
	if pattern == "" {
		err = fmt.Errorf("pattern is required")
		return
	}

	path := filepath.Clean(toolcall.ToolArgsValue(toolArgs, "path", context.ProjectRoot))
	caseInsensitive := toolcall.ToolArgsValue(toolArgs, "-i", false)
	enableMultiline := toolcall.ToolArgsValue(toolArgs, "multiline", false)
	showLineNumbers := toolcall.ToolArgsValue(toolArgs, "-n", true)
	fileType := toolcall.ToolArgsValue(toolArgs, "type", "")
	glob := toolcall.ToolArgsValue(toolArgs, "glob", "")
	afterLines := toolcall.ToolArgsValue(toolArgs, "-A", int64(0))
	beforeLines := toolcall.ToolArgsValue(toolArgs, "-B", int64(0))
	contextLines := toolcall.ToolArgsValue(toolArgs, "-C", int64(0))
	if contextLines > 0 {
		afterLines = contextLines
		beforeLines = contextLines
	}
	outputMode := toolcall.ToolArgsValue(toolArgs, "output_mode", "files_with_matches")
	headLimit := int(toolcall.ToolArgsValue(toolArgs, "head_limit", int64(0)))
	offset := int(toolcall.ToolArgsValue(toolArgs, "offset", int64(0)))
	matches, err := Ripgrep(ctx, pattern, path, glob, fileType, afterLines, beforeLines, caseInsensitive, enableMultiline)
	if err != nil {
		return
	}

	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Path < matches[j].Path
	})

	switch outputMode {
	case "content":
		matches = applyPagination(matches, offset, headLimit)
		result = formatContentMatches(matches, showLineNumbers)
	case "count":
		result = formatCountMatches(matches, offset, headLimit)
	case "files_with_matches":
		result = formatFileMatches(matches, offset, headLimit)
	default:
		result = formatFileMatches(matches, offset, headLimit)
	}
	return
}
