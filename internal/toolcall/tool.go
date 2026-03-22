// Package toolcall provides toolcall framework
package toolcall

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/sqlite"
)

// ToolDesc 表示一个工具
type ToolDesc struct {
	ID          int64
	Name        string
	Description string
	Category    string
	UsageCount  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// ToolUsage 表示工具使用记录
type ToolUsage struct {
	ID          int64
	ProjectPath string
	ToolID      int64
	UsedAt      time.Time
	Success     bool
	ErrorMsg    string
}

type ToolUsageStat struct {
	Name        string
	UsageCount  int
	SuccessRate float64
	LastUsed    time.Time
}

// Tool 定义可调用的工具
type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
	tokens   int      `json:"-"`
}

type Function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Strict      bool           `json:"strict,omitempty"`
	Parameters  map[string]any `json:"parameters"` // JSON Schema 对象
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolCallFunction `json:"function"`
}

type ToolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"` // JSON 字符串
}

var ToolDisplayName = &struct{}{}

// toolRegistry 工具注册表
var toolRegistry = map[string]ToolDef{}

func init() {
	sqlite.RegisterTableSchema(
		// 工具表
		`CREATE TABLE IF NOT EXISTS tools (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL,
			category TEXT,
			usage_count INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,

		// 工具使用记录表
		`CREATE TABLE IF NOT EXISTS tool_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_path TEXT NOT NULL,
			tool_id INTEGER NOT NULL,
			used_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			success BOOLEAN DEFAULT 1,
			error_msg TEXT,
			FOREIGN KEY (tool_id) REFERENCES tools(id) ON DELETE CASCADE
		)`,

		// 工具相关索引
		`CREATE INDEX IF NOT EXISTS idx_tools_category ON tools(category)`,
		`CREATE INDEX IF NOT EXISTS idx_tools_usage ON tools(usage_count DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_tool ON tool_usage(tool_id)`,
		`CREATE INDEX IF NOT EXISTS idx_tool_usage_time ON tool_usage(used_at DESC)`,
	)
}

func (t *Tool) GetTokens() int {
	if t.tokens != 0 {
		return t.tokens
	}

	b, err := json.Marshal(t)
	if err != nil { // panic if the tool can not be marshal.
		panic(err)
	}

	t.tokens = len([]rune(string(b))) / 2

	return t.tokens
}

func GetToolDisplayName(name string) string {
	words := strings.Split(name, "_")
	for i, word := range words {
		word = strings.ToUpper(word[0:1]) + word[1:]
		words[i] = word
	}
	return strings.Join(words, "")
}

// RegisterTool 注册工具
func RegisterTool(tool ToolDef) {
	name := tool.Name
	if _, ok := toolRegistry[name]; ok {
		panic(fmt.Sprintf("%s exists", name))
	}
	tool.DisplayName = GetToolDisplayName(name)
	toolRegistry[name] = tool
}

// GetAllTools 获取所有工具定义（用于API调用）
func GetAllTools(ctx context.Context) []Tool {
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
	if modelID == context.DeepseekReasoner {
		return nil
	}

	var tools []Tool
	for name, def := range toolRegistry {
		tools = append(tools, Tool{
			Type: "function",
			Function: Function{
				Name:        name,
				Description: def.Description,
				Parameters:  def.Parameters,
				Strict:      def.Strict,
			},
		})
	}
	return tools
}

// HandleToolCalls 处理工具调用（带统计）
func HandleToolCalls(ctx context.Context, tcs []ToolCall) (inputs []Message) {
	// 处理每个工具调用
	for _, tc := range tcs {
		id := tc.ID
		// 使用新的工具调用处理器
		result, err := HandleToolCall(ctx, tc.Function.Name, []byte(tc.Function.Arguments))
		if err != nil {
			// But we still need to tell the result to assistant
			result = err.Error()
		}
		input := Message{
			Role:       "tool",
			ToolCallID: id,
			Content:    result,
		}
		err = SaveMessages(ctx, input)
		if err != nil {
			outfmt.Debug("failed to save: %v", err)
		}
		inputs = append(inputs, input)

	}
	return inputs
}

// HandleToolCall 处理工具调用（带统计和超时）
func HandleToolCall(ctx context.Context, toolName string, argsRaw json.RawMessage) (string, error) {
	// 获取工具处理器
	tool, ok := toolRegistry[toolName]
	if !ok {
		return "", fmt.Errorf("未知工具: %s", toolName)
	}
	args := ToolArgs{}
	if err := json.Unmarshal(argsRaw, &args); err != nil {
		n := len(argsRaw)
		if n > 80 {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about raw argument tool %q received`+
				` which lead error:
- the length of the argument string: %d
- the last 40 bytes of the argument string: %q
- the first 40 bytes of the argument string: %q`, err, toolName, n,
				string(argsRaw[n-40:]), string(argsRaw[0:40]))
		} else {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about the raw argument tool %q received, 
which lead to the error:
- the length of the argument string：%d
- the argument raw：%q`, err, toolName, n, string(argsRaw))
		}
		return "", err
	}

	// 创建带超时的context（如果工具设置了超时）
	var cancel context.CancelFunc
	if tool.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, tool.Timeout)
		defer cancel()
	}

	ctx = context.WithValue(ctx, ToolDisplayName, tool.DisplayName)
	toolID, err := GetOrCreateTool(tool.Name, tool.Description, tool.Category)
	if err != nil {
		outfmt.Error(err.Error(), "name", tool.Name)
		// 继续执行工具，但不记录统计
		return tool.Handler(ctx, args)
	}

	// ✅ 新增：显示工具执行开始
	displayName := tool.DisplayName
	if displayName == "" {
		displayName = tool.Name
	}
	outfmt.Printf("🔄 正在执行 %s...\n", displayName)

	// 执行工具
	result, err := tool.Handler(ctx, args)

	// 检查是否超时
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("工具执行超时（%v）", tool.Timeout)
	}

	// ✅ 新增：立即显示执行结果
	if err != nil {
		outfmt.Printf("❌ %s 执行失败: %v\n", displayName, err)
	} else {
		outfmt.Printf("✅ %s 执行成功\n", displayName)
		// 如果结果简短，显示结果摘要
		if result != "" {
			// 清理结果，移除多余空白
			cleanResult := strings.TrimSpace(result)
			if len(cleanResult) > 0 {
				// 显示前200个字符作为摘要
				summary := TruncateString(cleanResult, 200)
				// 移除换行符，使输出更紧凑
				summary = strings.ReplaceAll(summary, "\n", " ")
				outfmt.Printf("   结果: %s\n", summary)
			}
		}
	}

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	if err := RecordToolUsage(ctx, toolID, success, errorMsg); err != nil {
		return "", err
	}

	// 截断工具结果，避免API调用失败
	if result != "" {
		result = TruncateToolResult(result)
	}

	return result, err
}

// GetOrCreateTool 获取或创建工具
func GetOrCreateTool(name, description, category string) (int64, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return 0, err
	}
	defer db.Close()
	var id int64
	err = db.QueryRow("SELECT id FROM tools WHERE name = ?", name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("查询工具失败: %w", err)
	}

	result, err := db.Exec(`
		INSERT INTO tools (name, description, category)
		VALUES (?, ?, ?)`, name, description, category)
	if err != nil {
		return 0, fmt.Errorf("创建工具失败: %w", err)
	}
	return result.LastInsertId()
}

// GetTool 根据ID获取工具
func GetTool(id int64) (*ToolDesc, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var tool ToolDesc
	err = db.QueryRow(`
		SELECT id, name, description, category, usage_count, created_at, updated_at
		FROM tools WHERE id = ?`, id).Scan(
		&tool.ID, &tool.Name, &tool.Description, &tool.Category,
		&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}
	return &tool, nil
}

// GetToolByName 根据名称获取工具
func GetToolByName(name string) (*ToolDesc, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var tool ToolDesc
	err = db.QueryRow(`
		SELECT id, name, description, category, usage_count, created_at, updated_at
		FROM tools WHERE name = ?`, name).Scan(
		&tool.ID, &tool.Name, &tool.Description, &tool.Category,
		&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}
	return &tool, nil
}

// ListTools 列出所有工具（可按分类过滤）
func ListTools(category string) ([]ToolDesc, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

	if category == "" {
		rows, err = db.Query(`
			SELECT id, name, description, category, usage_count, created_at, updated_at
			FROM tools ORDER BY usage_count DESC, name`)
	} else {
		rows, err = db.Query(`
			SELECT id, name, description, category, usage_count, created_at, updated_at
			FROM tools WHERE category = ? ORDER BY usage_count DESC, name`, category)
	}

	if err != nil {
		return nil, fmt.Errorf("查询工具失败: %w", err)
	}
	defer rows.Close()

	var tools []ToolDesc
	for rows.Next() {
		var tool ToolDesc
		if err := rows.Scan(
			&tool.ID, &tool.Name, &tool.Description, &tool.Category,
			&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt); err != nil {
			return nil, fmt.Errorf("扫描工具失败: %w", err)
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

// RecordToolUsage 记录工具使用
func RecordToolUsage(ctx context.Context, toolID int64, success bool, errorMsg string) error {
	projectRoot := context.ProjectRoot
	db, err := sqlite.OpenDB()
	if err != nil {
		return err
	}
	defer db.Close()
	// 更新工具使用次数
	_, err = db.Exec("UPDATE tools SET usage_count = usage_count + 1 WHERE id = ?", toolID)
	if err != nil {
		return fmt.Errorf("更新工具使用次数失败: %w", err)
	}

	// 记录使用详情
	_, err = db.Exec(`
		INSERT INTO tool_usage (project_path, tool_id, success, error_msg)
		VALUES (?, ?, ?, ?)`, projectRoot, toolID, success, errorMsg)
	if err != nil {
		return fmt.Errorf("记录工具使用详情失败: %w", err)
	}

	return nil
}

// GetToolUsageStats 获取工具使用统计
func GetToolUsageStats(days int) ([]ToolUsageStat, error) {
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

	query := `
		SELECT 
			t.name,
			t.usage_count,
			COALESCE(SUM(CASE WHEN tu.success THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 100) as success_rate,
			MAX(tu.used_at) as last_used
		FROM tools t
		LEFT JOIN tool_usage tu ON t.id = tu.tool_id
	`

	if days > 0 {
		query += " WHERE tu.used_at >= datetime('now', '-' || ? || ' days')"
		rows, err = db.Query(query+" GROUP BY t.id ORDER BY t.usage_count DESC", days)
	} else {
		rows, err = db.Query(query + " GROUP BY t.id ORDER BY t.usage_count DESC")
	}

	if err != nil {
		return nil, fmt.Errorf("查询工具统计失败: %w", err)
	}
	defer rows.Close()

	var stats []ToolUsageStat

	for rows.Next() {
		var stat ToolUsageStat
		var lastUsedStr sql.NullString
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &stat.SuccessRate, &lastUsedStr); err != nil {
			return nil, fmt.Errorf("扫描工具统计失败: %w", err)
		}
		if lastUsedStr.Valid && lastUsedStr.String != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastUsedStr.String); err == nil {
				stat.LastUsed = t
			}
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

// GetProjectToolUsage 获取项目工具使用情况
func GetProjectToolUsage(ctx context.Context, days int) ([]ToolUsageStat, error,
) {
	projectRoot := context.ProjectRoot
	db, err := sqlite.OpenDB()
	if err != nil {
		return nil, err
	}
	defer db.Close()
	var rows *sql.Rows

	query := `
		SELECT 
			t.name,
			COUNT(tu.id) as usage_count,
			MAX(tu.used_at) as last_used
		FROM tools t
		JOIN tool_usage tu ON t.id = tu.tool_id
		WHERE tu.project_path = ?
	`

	if days > 0 {
		query += " AND tu.used_at >= datetime('now', '-' || ? || ' days')"
		rows, err = db.Query(query+" GROUP BY t.id ORDER BY usage_count DESC", projectRoot, days)
	} else {
		rows, err = db.Query(query+" GROUP BY t.id ORDER BY usage_count DESC", projectRoot)
	}

	if err != nil {
		return nil, fmt.Errorf("查询项目工具使用失败: %w", err)
	}
	defer rows.Close()

	var stats []ToolUsageStat

	for rows.Next() {
		var stat ToolUsageStat
		var lastUsedStr sql.NullString
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &lastUsedStr); err != nil {
			return nil, fmt.Errorf("扫描项目工具使用失败: %w", err)
		}
		if lastUsedStr.Valid && lastUsedStr.String != "" {
			if t, err := time.Parse("2006-01-02 15:04:05", lastUsedStr.String); err == nil {
				stat.LastUsed = t
			}
		}
		stats = append(stats, stat)
	}
	return stats, nil
}

func Shuffle(in string) (out string) {
	runes := []rune(in)
	rand.Shuffle(len(runes), func(i, j int) {
		runes[i], runes[j] = runes[j], runes[i]
	})
	out = string(runes)
	return
}
