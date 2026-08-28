// Package toolcall provides toolcall framework
package toolcall

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dscli/dscli/internal/context"
	"github.com/dscli/dscli/internal/outfmt"
	"github.com/dscli/dscli/internal/prompt"
	"github.com/dscli/dscli/internal/roles"
	"github.com/dscli/dscli/internal/session"
	"github.com/dscli/dscli/internal/sqlite"
	"github.com/nanjj/clog"
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

	// toolAliases 别名映射：alias -> 主名（旧名兼容，如 vision_file_upload）。
	toolAliases = map[string]string{}

	// toolRegistryRWMutex tool registry rwmutex
	toolRegistryRWMutex = sync.RWMutex{}

	// DispatchMCP dispatches a tool call to an MCP server.
	// Set by the mcphub package init (via web package init).
	// If nil, unknown tools fall back to the default error message.
	DispatchMCP func(ctx context.Context, toolName, argsRaw string) (result, warning string, err error)
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
func RegisterTool(tool ToolDef) error {
	toolRegistryRWMutex.Lock()
	defer toolRegistryRWMutex.Unlock()
	name := tool.Name
	if _, ok := toolRegistry[name]; ok {
		return fmt.Errorf("tool %q already registered", name)
	}
	for _, alias := range tool.Aliases {
		if _, ok := toolRegistry[alias]; ok {
			return fmt.Errorf("tool alias %q already registered", alias)
		}
		if _, ok := toolAliases[alias]; ok {
			return fmt.Errorf("tool alias %q already registered", alias)
		}
	}
	tool.DisplayName = GetToolDisplayName(name)
	toolRegistry[name] = tool
	for _, alias := range tool.Aliases {
		toolAliases[alias] = name
	}
	return nil
}

func GetToolDef(ctx context.Context, toolName string) (tool ToolDef, ok bool) {
	span, ctx := clog.StartSpanFromContext(ctx, "GetToolDef")
	defer span.Finish()
	toolRegistryRWMutex.RLock()
	defer toolRegistryRWMutex.RUnlock()
	tool, ok = toolRegistry[toolName]
	if !ok {
		// 别名解析：旧名（如 vision_file_upload）映射到主名。
		if canonical, has := toolAliases[toolName]; has {
			tool, ok = toolRegistry[canonical]
		}
	}
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

// roleToolsSpec returns the role's tools spec ("all", "", or "a,b") from
// role_configs, falling back to roles.DefaultFor. It is the single source
// of truth for "which tools this role may use": GetAllTools (dscli chat /
// ask) and the DSML executor (webchat) both derive their tool set from it,
// so the web model can only ever call what the role is configured for.
func roleToolsSpec(ctx context.Context, role string) string {
	sessionID := session.GetCurrentSessionID(ctx)
	if cfg, err := roles.GetRoleConfig(ctx, role, sessionID); err == nil && cfg != nil {
		return cfg.Tools
	}
	return roles.DefaultFor(role).Tools
}

// roleToolAllowSet converts the role's tools spec into an allow-set:
// nil = everything ("all"), empty map = nothing, non-empty map = those names.
func roleToolAllowSet(ctx context.Context, role string) map[string]bool {
	return allowSetFromSpec(roleToolsSpec(ctx, role))
}

// allowSetFromSpec converts a stored spec ("all", "", "a,b") into an
// allow-set: nil = everything ("all"), empty non-nil map = explicitly
// nothing, non-empty map = only those names. Shared by the tools allowlist;
// skills filtering uses roles.ParseSkillsList directly (same shape).
func allowSetFromSpec(spec string) map[string]bool {
	names := roles.ParseToolsList(spec)
	if names == nil {
		return nil // "all"
	}
	if len(names) == 0 {
		return map[string]bool{} // none
	}
	allowSet := make(map[string]bool, len(names))
	for _, t := range names {
		allowSet[t] = true
	}
	return allowSet
}

// GetAllTools returns tools available for the current role.
// Filters tools by role config from DB; falls back to role defaults
// (roles.DefaultFor) when no config row exists.
func GetAllTools(ctx context.Context) []Tool {
	span, ctx := clog.StartSpanFromContext(ctx, "GetAllTools")
	defer span.Finish()

	role := context.ContextValue(ctx, context.CurrentRoleKey, "dev")
	// dev is capable by default; expert/review/test execute nothing until
	// configured. This mirrors the role list display, which shows the very
	// same defaults (roleToolsSpec).
	allowSet := roleToolAllowSet(ctx, role)
	if allowSet != nil && len(allowSet) == 0 {
		return nil // explicit empty = no tools
	}

	toolRegistryRWMutex.RLock()
	defer toolRegistryRWMutex.RUnlock()

	names := slices.Collect(maps.Keys(toolRegistry))
	slices.Sort(names)
	var tools []Tool
	for _, name := range names {
		def, ok := toolRegistry[name]
		if !ok {
			continue
		}
		if allowSet != nil {
			// Accept the canonical name or a legacy spelling in the spec
			// (e.g. "exec_command" for the shell tool), mirroring the DSML
			// executor's allow-set check - one spec, one meaning.
			matched := allowSet[name]
			if !matched {
				for legacy, native := range dsmlLegacyNames {
					if native == name && allowSet[legacy] {
						matched = true
						break
					}
				}
			}
			if !matched {
				continue
			}
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

// ToolCallOutcome 是一次工具调用的完整结果：供协议使用的 tool 消息，以及
// 结构化的 result/warning/error 内容。HandleToolCalls 只返回消息（chat 路径），
// DSML 执行器需要结构化内容来构造 <tool_result> JSON，二者共用执行内核。
type ToolCallOutcome struct {
	Message prompt.Message
	Content ToolContent
}

// HandleToolCalls 处理工具调用（带统计）。
// 多个工具调用时先打印汇总行，并为每个调用显示序号。
// 执行期间注册中断处理：用户主动停止（Ctrl+C/kill）时先标记
// 未完成的调用再退出，避免下次启动重放已取消的操作。
func HandleToolCalls(ctx context.Context, tcs []prompt.ToolCall) (inputs []prompt.Message) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleToolCalls")
	defer span.Finish()

	stop := InstallInterruptHandler(ctx)
	defer stop()
	toolProgress.Store(0)

	// 多个工具调用时先打印汇总行，便于区分并行调用
	if len(tcs) > 1 {
		outfmt.Printf("📋 本轮共 %d 个工具调用\n", len(tcs))
	}

	outcomes, dualUsers := executeToolCalls(ctx, tcs, true)
	if len(outcomes) == 0 && len(dualUsers) == 0 {
		return nil // 保持原契约：空输入返回 nil，而非非 nil 空 slice
	}
	inputs = make([]prompt.Message, 0, len(outcomes)+len(dualUsers))
	for _, o := range outcomes {
		inputs = append(inputs, o.Message)
	}
	// 双消息 user 消息统一追加在全部 tool 消息之后（顺序：tool × N → user × N）。
	inputs = append(inputs, dualUsers...)
	return inputs
}

// executeToolCalls 是 HandleToolCalls 的执行内核，DSML 执行器同样复用：
// 逐条执行工具、记录使用统计、截断结果、拆分双消息，按顺序返回每个调用的
// 结构化结果（ToolCallOutcome，与 tcs 一一对应）。
//
// save 控制是否把 tool 消息写入消息表：
//   - true（chat 主路径）：落库后中断恢复可判断已完成调用，结果已落库才
//     推进 toolProgress；
//   - false（DSML/webchat 路径）：不落库。web 会话发生在浏览器里，把工具
//     结果写进当前会话的 messages 表会破坏 assistant↔tool 配对——主会话的
//     CleanupReverse 按 tool_calls 数量与 tool 消息数量配对，多出的 DSML
//     tool 消息会让整个 ask_expert 轮次（包括其真实结果）从历史中被裁掉。
//
// 中断处理不在本函数内安装：DSML 路径运行在 chat 的 HandleToolCalls 内部，
// 重复注册会收到同一次 Ctrl+C 并各自插入占位消息（同一调用得到两条占位
// tool 消息，反而破坏配对）；外层 handler 已覆盖整个 ask_expert 调用。
// 双消息（DualMessage）拆分对 DSML 无意义：DSML 的 <tool_result> 封装
// 无法携带附加 user 消息（且 webchat 模型看不到图像块），非落库模式下
// 附加 user 消息由调用方丢弃并告警（见 ExecuteDSMLToolCalls）。
func executeToolCalls(ctx context.Context, tcs []prompt.ToolCall, save bool) (outcomes []ToolCallOutcome, dualUsers []prompt.Message) {
	dualUsers = []prompt.Message{}
	outcomes = make([]ToolCallOutcome, 0, len(tcs))

	// 处理每个工具调用
	for i, tc := range tcs {
		id := tc.ID
		// 使用新的工具调用处理器
		result, user, err := handleToolCall(ctx, tc.Function.Name, tc.Function.Arguments, i+1, len(tcs))

		toolResult, userMsg, isDual := SplitDualResult(result)
		if isDual {
			result = toolResult
		}

		content := ToolContent{
			Index:    i + 1,
			ToolName: tc.Function.Name,
			Result:   result,
			Error:    Error(err),
			Warning:  user,
		}

		input := prompt.Message{
			Role:       "tool",
			ToolCallID: id,
			Content:    content.String(),
		}

		if save {
			saveErr := prompt.SaveMessages(ctx, input)
			if saveErr != nil {
				outfmt.Debug("failed to save: %v", saveErr)
			} else {
				// 结果已落库才算完成：中断标记据此判断未执行的调用，
				// 插入占位 tool 消息（不裁剪 tool_calls）。
				toolProgress.Store(int64(i + 1))
			}
		}

		if isDual && userMsg != nil {
			dualUsers = append(dualUsers, *userMsg)
		}
		outcomes = append(outcomes, ToolCallOutcome{Message: input, Content: content})
	}

	// 所有 tool 消息之后统一追加双消息 user 消息（顺序：tool × N → user × N）。
	if save {
		for _, m := range dualUsers {
			if userSaveErr := prompt.SaveMessages(ctx, m); userSaveErr != nil {
				outfmt.Debug("failed to save dual user message: %v", userSaveErr)
			}
		}
	}
	return outcomes, dualUsers
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

// toolCallIndexPrefix 生成工具调用的序号前缀。
// total > 1 时形如 "[1/2] "，单个调用返回空串，避免噪音。
func toolCallIndexPrefix(index, total int) string {
	if index > 0 && total > 1 {
		return fmt.Sprintf("[%d/%d] ", index, total)
	}
	return ""
}

// HandleToolCall 处理单个工具调用（带统计和超时）。
// 以 index=0, total=0 调用内部实现，不显示序号；
// 批量调用请使用 HandleToolCalls。
func HandleToolCall(ctx context.Context, toolName, argsRaw string) (result, warning string, err error) {
	return handleToolCall(ctx, toolName, argsRaw, 0, 0)
}

// handleToolCall 是 HandleToolCall 的内部实现。
// index/total 用于在显示中标注 "[i/total]" 序号。
func handleToolCall(ctx context.Context, toolName, argsRaw string, index, total int) (result, warning string, err error) {
	span, ctx := clog.StartSpanFromContext(ctx, "HandleToolCall")
	defer span.Finish()
	// 获取工具处理器
	tool, ok := GetToolDef(ctx, toolName)
	if !ok {
		// Not a registered dscli tool — try MCP dispatch.
		if DispatchMCP != nil {
			return DispatchMCP(ctx, toolName, argsRaw)
		}
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
keep each part around 300 lines. After write the first part,
use write_file with insert_before_line=<total+1> to append the left
parts one by one IN ORDER (read_file first to get the total, or use
the returned context window). DO NOT MISORDER! THIS UNMARSHAL
CONTENT WILL BE DISCARD!
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

	toolID, err := GetOrCreateTool(ctx, tool.Name, tool.Description, tool.Category)
	if err != nil {
		outfmt.Error(err.Error(), "name", tool.Name)
		// 继续执行工具，但不记录统计
		return tool.Handler(ctx, args)
	}

	// 显示工具执行开始
	displayName := tool.DisplayName
	if displayName == "" {
		displayName = tool.Name
	}
	indexPrefix := toolCallIndexPrefix(index, total)
	outfmt.Printf("🔄 %s正在执行 %s...\n", indexPrefix, displayName)

	// 执行工具
	result, warning, err = tool.Handler(ctx, args)

	// 检查是否超时。用实际生效的 timeout 变量（可能是工具参数传入的，
	// 不一定是 tool.Timeout 默认值），否则长超时任务失败时错误信息会误导。
	if ctx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("工具执行超时（%v）", timeout)
	}

	// 立即显示执行结果
	if err != nil {
		outfmt.Printf("❌ %s%s 执行失败: %v\n", indexPrefix, displayName, err)
	} else {
		outfmt.Printf("✅ %s%s 执行成功\n", indexPrefix, displayName)
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
func GetOrCreateTool(ctx context.Context, name, description, category string) (int64, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "GetOrCreateTool")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return 0, err
	}
	defer db.Close(ctx)
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
func GetTool(ctx context.Context, id int64) (*ToolDesc, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "GetTool")
	defer span.Finish()
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)
	var tool ToolDesc
	err = db.QueryRow(`
		SELECT id, name, description, category, usage_count, created_at, updated_at
		FROM tools WHERE id = ?`, id).Scan(
		&tool.ID, &tool.Name, &tool.Description, &tool.Category,
		&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}
	return &tool, nil
}

// GetToolByName 根据名称获取工具
func GetToolByName(ctx context.Context, name string) (*ToolDesc, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "GetToolByName")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)
	var tool ToolDesc
	err = db.QueryRow(`
		SELECT id, name, description, category, usage_count, created_at, updated_at
		FROM tools WHERE name = ?`, name).Scan(
		&tool.ID, &tool.Name, &tool.Description, &tool.Category,
		&tool.UsageCount, &tool.CreatedAt, &tool.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("获取工具失败: %w", err)
	}
	return &tool, nil
}

// ListTools 列出所有工具（可按分类过滤）。
// 以运行时注册表为权威来源，合并 DB 中的使用统计。
func ListTools(ctx context.Context, category string) ([]ToolDesc, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "ListTools")
	defer span.Finish()

	// 1. 从 DB 获取使用统计
	dbStats := map[string]ToolDesc{}
	if db, err := sqlite.OpenDB(ctx); err == nil {
		func() {
			defer db.Close(ctx)
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

	// 4. 按分类分组，分类内按使用次数降序、名称升序排序
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
	span, ctx := clog.StartSpanFromContext(ctx, "RecordToolUsage")
	defer span.Finish()

	projectRoot := context.ProjectRoot
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
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
func GetToolUsageStats(ctx context.Context, days int) ([]ToolUsageStat, error) {
	span, ctx := clog.StartSpanFromContext(ctx, "GetToolUsageStats")
	defer span.Finish()

	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}

	defer db.Close(ctx)
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
	span, ctx := clog.StartSpanFromContext(ctx, "GetProjectToolUsage")
	defer span.Finish()

	projectRoot := context.ProjectRoot
	db, err := sqlite.OpenDB(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close(ctx)
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
