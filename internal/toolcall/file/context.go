package file

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

const (
	// contextBefore 编辑区域前展示的行数
	contextBefore = 5
	// contextAfter 编辑区域后展示的行数
	contextAfter = 8
	// contextMaxAll 编辑区域全量展示的最大行数
	contextMaxAll = 36
	// contextHead 大编辑时头部展示行数
	contextHead = 18
	// contextTail 大编辑时尾部展示行数
	contextTail = 18
)

// contextReadLines 读取文件所有行
func contextReadLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines, scanner.Err()
}

// AppendEditContext 生成行范围编辑后的上下文窗口。
//
// startLine/endLine 是原编辑范围的 1-based 包含行号；
// oldReplaced 是被替换的原始行数（endLine - startLine + 1）；
// newLineCount 是替换后的新行数。
//
// 参考 ds4 agent_edit_result_append_context 的设计：
//   - 编辑前 5 行 + 编辑后 8 行非对称窗口（下游更重要）
//   - 编辑块 ≤36 行全量显示，>36 行首尾各 18 行截断
//   - 行偏移警告（行数变化时）
func AppendEditContext(path string, startLine, endLine, oldReplaced, newLineCount int) string {
	lines, err := contextReadLines(path)
	if err != nil || len(lines) == 0 {
		return ""
	}
	totalLines := len(lines)

	// 锚点：新内容在文件中的实际位置
	anchorStart := startLine
	anchorEnd := startLine + newLineCount - 1
	anchorEnd = max(anchorEnd, anchorStart)
	anchorStart = min(anchorStart, totalLines)
	anchorEnd = min(anchorEnd, totalLines)

	// 上下文窗口边界
	ctxStart := max(anchorStart-contextBefore, 1)
	ctxEnd := min(anchorEnd+contextAfter, totalLines)

	var sb strings.Builder
	sb.WriteString("\n--- 编辑后上下文 ---\n")

	// 行偏移警告
	delta := newLineCount - oldReplaced
	if delta != 0 {
		fmt.Fprintf(&sb,
			"⚠️ 行数变化：从第 %d 行起偏移 %+d（原第 %d 行现在是第 %d 行）。依赖旧行号前请重新读取。\n",
			endLine+1, delta, endLine+1, endLine+1+delta)
	}

	fmt.Fprintf(&sb, "📄 %s 第 %d-%d 行（共 %d 行）:\n",
		path, ctxStart, ctxEnd, totalLines)

	editedLines := anchorEnd - anchorStart + 1
	if editedLines <= contextMaxAll {
		// 编辑块较小，全量展示
		for i := ctxStart; i <= ctxEnd; i++ {
			fmt.Fprintf(&sb, "%d: %s\n", i, lines[i-1])
		}
	} else {
		// 大编辑：首尾截断
		headEnd := anchorStart + contextHead - 1
		tailStart := anchorEnd - contextTail + 1
		if tailStart <= headEnd {
			tailStart = headEnd + 1
		}

		for i := ctxStart; i <= headEnd; i++ {
			fmt.Fprintf(&sb, "%d: %s\n", i, lines[i-1])
		}
		fmt.Fprintf(&sb, "... %d 行省略 ...\n", tailStart-headEnd-1)
		for i := tailStart; i <= ctxEnd; i++ {
			fmt.Fprintf(&sb, "%d: %s\n", i, lines[i-1])
		}
	}

	return sb.String()
}

// AppendWriteFileContext 生成全文件写入后的上下文窗口。
//
// 小文件（≤36 行）全量展示；大文件首尾各 18 行。
func AppendWriteFileContext(path string) string {
	lines, err := contextReadLines(path)
	if err != nil {
		return ""
	}
	totalLines := len(lines)
	if totalLines == 0 {
		return "\n--- 写入后 ---\n文件为空"
	}

	var sb strings.Builder
	if totalLines <= contextMaxAll {
		fmt.Fprintf(&sb, "\n--- 写入后：%s（%d 行）---\n", path, totalLines)
		for i, line := range lines {
			fmt.Fprintf(&sb, "%d: %s\n", i+1, line)
		}
		return sb.String()
	}

	fmt.Fprintf(&sb, "\n--- 写入后：%s（%d 行，显示首尾各 %d 行）---\n",
		path, totalLines, contextHead)
	for i := 0; i < contextHead && i < totalLines; i++ {
		fmt.Fprintf(&sb, "%d: %s\n", i+1, lines[i])
	}
	fmt.Fprintf(&sb, "... %d 行省略 ...\n", totalLines-contextHead-contextTail)
	for i := totalLines - contextTail; i < totalLines; i++ {
		fmt.Fprintf(&sb, "%d: %s\n", i+1, lines[i])
	}
	return sb.String()
}
