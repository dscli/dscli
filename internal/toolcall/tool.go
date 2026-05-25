// Package toolcall provides toolcall framework
package toolcall

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"gitcode.com/dscli/dscli/internal/context"
	"gitcode.com/dscli/dscli/internal/outfmt"
	"gitcode.com/dscli/dscli/internal/prompt"
	"gitcode.com/dscli/dscli/internal/roles"
	"gitcode.com/dscli/dscli/internal/session"
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

var (
	// toolRegistry 工具注册表
	toolRegistry = map[string]ToolDef{}

	// toolRegistryRWMutex tool registry rwmutex
	toolRegistryRWMutex = sync.RWMutex{}
)

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
	toolRegistryRWMutex.Lock()
	defer toolRegistryRWMutex.Unlock()
	name := tool.Name
	if _, ok := toolRegistry[name]; ok {
		panic(fmt.Sprintf("%s exists", name))
	}
	tool.DisplayName = GetToolDisplayName(name)
	toolRegistry[name] = tool
}

func GetToolDef(ctx context.Context, toolName string) (tool ToolDef, ok bool) {
	toolRegistryRWMutex.RLock()
	defer toolRegistryRWMutex.RUnlock()
	tool, ok = toolRegistry[toolName]
	return tool, ok
}

// KnownToolNames returns all registered tool names from the in-memory registry.
func KnownToolNames() []string {
	toolRegistryRWMutex.RLock()
	defer toolRegistryRWMutex.RUnlock()
	names := make([]string, 0, len(toolRegistry))
	for name := range toolRegistry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// GetAllTools 获取所有工具定义（用于API调用）
// GetAllTools returns tools available for the current role.
// Filters tools by role config from DB; falls back to hardcoded:
// dev gets all, others get none.
func GetAllTools(ctx context.Context) []Tool {
	role := context.ContextValue(ctx, context.CurrentRoleKey, "dev")
	modelID := context.ContextValue(ctx, context.CurrentModelIDKey, context.DeepseekChat)
	if modelID == context.DeepseekReasoner {
		return nil
	}
	// Determine which tools to include
	var allowSet map[string]bool // nil = all, non-nil = filter

	sessionID := session.GetCurrentSessionID(ctx)
	cfg, _ := roles.GetRoleConfig(role, sessionID)
	if cfg == nil {
		// Fallback: only dev gets tools
		if role != "dev" {
			return nil
		}
		// allowSet remains nil → return all
	} else {
		allowedTools := roles.ParseToolsList(cfg.Tools)
		if allowedTools != nil {
			if len(allowedTools) == 0 {
				return nil // explicit empty = no tools
			}
			allowSet = make(map[string]bool, len(allowedTools))
			for _, t := range allowedTools {
				allowSet[t] = true
			}
		}
		// allowSet stays nil for "all"
	}

	toolRegistryRWMutex.RLock()
	defer toolRegistryRWMutex.RUnlock()

	var tools []Tool
	for name, def := range toolRegistry {
		if allowSet != nil && !allowSet[name] {
			continue
		}
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
func HandleToolCalls(ctx context.Context, tcs []prompt.ToolCall) (inputs []prompt.Message) {
	// 处理每个工具调用
	for i, tc := range tcs {
		id := tc.ID
		// 使用新的工具调用处理器
		result, user, err := HandleToolCall(ctx, tc.Function.Name, tc.Function.Arguments)
		toolContent := ToolContent{
			Index:    i + 1,
			ToolName: tc.Function.Name,
			Result:   result,
			Error:    Error(err),
			Warning:  user,
		}

		input := prompt.Message{
			Role:       "tool",
			ToolCallID: id,
			Content:    toolContent.String(),
		}

		saveErr := prompt.SaveMessages(ctx, input)

		if saveErr != nil {
			outfmt.Debug("failed to save: %v", err)
		}
		inputs = append(inputs, input)

	}
	return inputs
}

func FixBrokenJSON(broken string) (result string) {
	if len(broken) == 0 {
		return "{}"
	}

	if len(broken) < 3 {
		result = broken
		return result
	}
	result = broken
	lastCh := broken[len(broken)-1]
	lastCh2 := broken[len(broken)-2]
	lastCh3 := broken[len(broken)-3]
	// no closing curly brace
	if lastCh == '"' && lastCh2 != '\\' {
		result += "}"
		return result
	}

	//  fake right closing curly brace
	if lastCh == '}' && lastCh2 != '"' && lastCh3 != '\\' {
		result += "\"}"
		return result
	}

	// fake right quote
	if lastCh == '"' && lastCh2 == '\\' {
		result += "\"}"
		return result
	}

	if lastCh == '}' && lastCh2 == '"' && lastCh3 != '\\' {
		return result
	}

	if lastCh == '\\' && lastCh2 != '\\' {
		result = result[0 : len(result)-1]
		result += "\"}"
		return result
	}
	result += "\"}"
	return result
}

// HandleToolCall 处理工具调用（带统计和超时）
func HandleToolCall(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	// 获取工具处理器
	tool, ok := GetToolDef(ctx, toolName)
	if !ok {
		err = fmt.Errorf("未知工具: %s", toolName)
		warning = fmt.Sprintf("所调用工具 %q 不存在，请严格按照 tools 列表所提供工具调用", toolName)
		outfmt.Println(warning)
		return result, warning, err
	}

	truncated := context.ContextValue(ctx, context.FinishReasonLengthKey, false)
	args := ToolArgs{}
	if truncated {
		outfmt.Printf("JSON消息已截断: %s\n", TruncateHeadTail(argsRaw, 100))
		argsRaw = FixBrokenJSON(argsRaw)
		outfmt.Printf("JSON消息已修复: %s\n", TruncateHeadTail(argsRaw, 100))
	}
	if err := json.Unmarshal([]byte(argsRaw), &args); err != nil {
		n := len(argsRaw)
		if n > 80 {
			input := string(argsRaw)
			notice := fmt.Sprintf(`
--------IMPORTANT-------NOTICE-----IMPORTANT----------
Looks you are using write_file tool to write large file
(around %d characters), you can seperate the file into several parts,
 keep each part around 300 lines, after write the first part, 
use append = true to append the left parts one by one IN ORDER. 
DO NOT MISORDER! THIS UNMARSHAL CONTENT WILL BE DISCARD!
-------------------------NOTICE------------------------`, n)
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about raw argument tool %q received`+
				` which lead error:
- the length of the argument string: %d
- the last 40 bytes of the argument string: %q
- the first 40 bytes of the argument string: %q

%s`, err, toolName, n,
				TruncateHead(input, 40), TruncateTail(input, 40), notice)
		} else {
			err = fmt.Errorf(`failed to unmarshal arguments: %w, below `+
				`is the details about the raw argument tool %q received, 
which lead to the error:
- the length of the argument string：%d
- the argument raw：%q`, err, toolName, n, string(argsRaw))
		}
		return "", "", err
	}

	seconds := ToolArgsValue(args, "timeout", int64(0))
	var timeout time.Duration
	if seconds > int64(0) {
		timeout = time.Second * time.Duration(seconds)
	}
	if timeout <= 0 {
		timeout = tool.Timeout
	}

	// 创建带超时的context（如果工具设置了超时）
	var cancel context.CancelFunc
	if timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	ctx = context.WithValue(ctx, context.ToolDisplayNameKey, tool.DisplayName)
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
	result, warning, err = tool.Handler(ctx, args)

	// 检查是否超时
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("工具执行超时（%v）", tool.Timeout)
	}

	// ✅ 新增：立即显示执行结果
	if err != nil {
		outfmt.Printf("❌ %s 执行失败: %v\n", displayName, err)
	} else {
		outfmt.Printf("✅ %s 执行成功\n", displayName)
	}

	// 记录使用情况
	success := err == nil
	errorMsg := ""
	if err != nil {
		errorMsg = err.Error()
	}

	if recordErr := RecordToolUsage(ctx, toolID, success, errorMsg); recordErr != nil {
		outfmt.Error("failed to record tool usage: %v", recordErr)
	}

	// 截断工具结果，避免API调用失败
	if result != "" {
		result = TruncateToolResult(result)
	}

	return result, warning, err
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

// ListTools 列出所有工具（可按分类过滤）。
// 以运行时注册表为权威来源，合并 DB 中的使用统计。
func ListTools(category string) ([]ToolDesc, error) {
	// 1. 从 DB 获取使用统计
	dbStats := map[string]ToolDesc{}
	if db, err := sqlite.OpenDB(); err == nil {
		func() {
			defer db.Close()
			rows, err := db.Query(`SELECT id, name, description, category, usage_count, created_at, updated_at FROM tools`)
			if err != nil {
				return
			}
			defer rows.Close()
			for rows.Next() {
				var t ToolDesc
				if err := rows.Scan(&t.ID, &t.Name, &t.Description, &t.Category,
					&t.UsageCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
					continue
				}
				dbStats[t.Name] = t
			}
		}()
	}

	// 2. 以注册表为准生成列表
	toolRegistryRWMutex.RLock()
	defer toolRegistryRWMutex.RUnlock()

	var tools []ToolDesc
	for name, def := range toolRegistry {
		if category != "" && def.Category != category {
			continue
		}
		td := ToolDesc{
			Name:        name,
			Description: def.Description,
			Category:    def.Category,
		}
		if db, ok := dbStats[name]; ok {
			td.ID = db.ID
			td.UsageCount = db.UsageCount
			td.CreatedAt = db.CreatedAt
			td.UpdatedAt = db.UpdatedAt
		}
		tools = append(tools, td)
	}

	// 3. 按分类分组，分类内按使用次数降序、名称升序排序
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Category != tools[j].Category {
			return tools[i].Category < tools[j].Category
		}
		if tools[i].UsageCount != tools[j].UsageCount {
			return tools[i].UsageCount > tools[j].UsageCount
		}
		return tools[i].Name < tools[j].Name
	})

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
			COALESCE(SUM(CASE WHEN tu.success THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 100) as success_rate,
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
		if err := rows.Scan(&stat.Name, &stat.UsageCount, &stat.SuccessRate, &lastUsedStr); err != nil {
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
