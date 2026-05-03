package prompt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/session"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

// MaxNoteContentLen 笔记内容最大长度（rune），供 tool handler 参考
const MaxNoteContentLen = 40

// defaultNoteDays 加载笔记默认天数
const defaultNoteDays = 30

// Note 对话笔记，用于跨对话记忆
type Note struct {
	ID        int64
	SessionID int64
	Content   string
	CreatedAt time.Time
}

func init() {
	sqlite.RegisterTableSchema(
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			content TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY (session_id) REFERENCES sessions(id)
		)`,
	)
}

// SaveNote 保存一条对话笔记（限制 MaxNoteContentLen 字以内）
func SaveNote(ctx context.Context, content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return fmt.Errorf("笔记内容不能为空")
	}
	// 按 rune 截断
	runes := []rune(content)
	if len(runes) > MaxNoteContentLen {
		content = string(runes[:MaxNoteContentLen])
	}

	sessionID := session.GetCurrentSessionID(ctx)
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.ExecContext(ctx,
		`INSERT INTO notes (session_id, content) VALUES (?, ?)`,
		sessionID, content)
	if err != nil {
		return fmt.Errorf("保存笔记失败: %w", err)
	}
	return nil
}

// LoadNotes 加载当前项目最近N天的笔记，最多返回10条
func LoadNotes(ctx context.Context, days int) ([]Note, error) {
	sessionID := session.GetCurrentSessionID(ctx)
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx,
		`SELECT id, content, created_at FROM notes
		 WHERE session_id = ? AND created_at >= ?
		 ORDER BY created_at DESC LIMIT 10`,
		sessionID, time.Now().AddDate(0, 0, -days).Format("2006-01-02 15:04:05"))
	if err != nil {
		return nil, fmt.Errorf("查询笔记失败: %w", err)
	}
	defer rows.Close()

	var notes []Note
	for rows.Next() {
		var n Note
		if err := rows.Scan(&n.ID, &n.Content, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("扫描笔记失败: %w", err)
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历笔记失败: %w", err)
	}
	return notes, nil
}

// FormatTime 格式化时间为简短形式
func FormatTime(t time.Time) string {
	now := time.Now()
	if t.Year() == now.Year() && t.YearDay() == now.YearDay() {
		return t.Format("15:04")
	}
	if t.Year() == now.Year() {
		return t.Format("01-02 15:04")
	}
	return t.Format("2006-01-02 15:04")
}

// BuildNotePrompt 从近期笔记构建记忆线索提示词
func BuildNotePrompt(ctx context.Context) string {
	if notes, noteErr := LoadNotes(ctx, defaultNoteDays); noteErr == nil && len(notes) > 0 {
		var b strings.Builder
		b.WriteString("## 📝 近期对话笔记\n")
		b.WriteString("以下是此前对话中记录的关键摘要，可作为回忆线索：\n\n")
		for _, n := range notes {
			fmt.Fprintf(&b, "- %s: %s\n", FormatTime(n.CreatedAt), n.Content)
		}
		b.WriteString("\n如需更多细节，可使用 recall 工具搜索完整对话历史。")
		return b.String()
	}
	return ""
}

func HandleNote(ctx context.Context, content string) (result string, suggestion string, err error) {
	// 警告超过限制（实际 SaveNote 也会截断）
	if len([]rune(content)) > MaxNoteContentLen {
		suggestion = fmt.Sprintf("笔记超过%d字已自动截断。下次请控制在%d字以内。",
			MaxNoteContentLen, MaxNoteContentLen)
	}

	if saveErr := SaveNote(ctx, content); saveErr != nil {
		err = saveErr
		return
	}

	result = "笔记已保存。"
	return
}
