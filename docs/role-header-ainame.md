# 角色头叠加 Ainame 身份

## 目标

`--role dev/expert/review/test/architect` 会话中，AI 消息的角色头在保留角色标签的同时，前置当前会话的 Ainame 身份（名字 + 邮箱 + bird/frog 图标）。

目标输出格式（以 architect + 玻尔为例）：

```
🐦 玻尔 <bohr@dscli.io> 🏗️ architect·软件架构师 ················ 12:43:10 🕐
```

对照现状（只有角色标签，无名字/邮箱）：

```
🏗️ architect·软件架构师 ······································· 12:43:10 🕐
```

## 验收标准（用户已确认）

1. **范围**：所有角色头统一生效（dev/expert/review/test/architect — 共享同一渲染路径）。
2. **缺失回退**：会话无 Ainame（`AINameCNKey` 或 `AINameEmailKey` 为空）时，保持现状纯角色头（如 `🏗️ architect·软件架构师`）。
3. **图标**：content 头行首 = Ainame 的 bird/frog 图标（🐦/🐸；无则保持现有默认）；角色标签 = 角色图标 + 角色名（`🏗️ architect·软件架构师`），完整格式为：

   `bird/frog图标 名字 <邮箱> 🏗️ architect·软件架构师 ········ 时间 🕐`

4. **reasoning 头对称处理**：`💭` reasoning 头同样叠加 Ainame，格式为
   `💭 玻尔 <bohr@dscli.io> 🏗️ architect·软件架构师 ····`。reasoning 头行首保持 `💭`（与纯聊天路径 `💭 玻尔 <...>` 逐字对称），**不显示** bird/frog 图标；bird/frog 图标仅用于 content 头行首。

## 现状（代码事实）

全部改动集中在 `internal/outfmt/output.go`，角色头渲染仅此一处（streaming 模式 content 头由同一 `PrintContent` 的分支控制）。

### formatChatHeader（output.go:500）

```go
func formatChatHeader(icon, nameCN, email, now string) string {
	left := icon + " " + nameCN
	if email != "" {
		left += " <" + email + ">"
	}
	right := now + " 🕐"

	leftW := runewidth.StringWidth(left)
	rightW := runewidth.StringWidth(right)

	// 当左侧身份信息过长时截断，确保整行不超过 headerLineWidth。
	maxLeftW := headerLineWidth - rightW - 4 // 4 = " ·· " at minimum
	if leftW > maxLeftW && maxLeftW >= 5 {
		left = runewidth.Truncate(left, maxLeftW, "…")
		leftW = runewidth.StringWidth(left)
	}

	padding := max(headerLineWidth-leftW-rightW, 2)

	return left + " " + strings.Repeat("·", padding-2) + " " + right
}
```

`headerLineWidth = 80`（output.go:493）。截断逻辑已就绪，自动处理加长后的行宽。

### PrintContent 角色头分支（output.go:591-640）

```go
// 角色头：--role dev/expert/review/test/architect 时用角色身份替代 AI 名。
role := context.ContextValue(ctx, context.CurrentRoleKey, "")
disp := roles.DisplayFor(role)
useRoleHeader := role != ""

// AI name for header
nameCN := context.ContextValue(ctx, context.AINameCNKey, "")
email := context.ContextValue(ctx, context.AINameEmailKey, "")
birdFrog := context.ContextValue(ctx, context.AINameBirdFrogKey, "")
now := time.Now().Local().Format(time.TimeOnly)

// 根据 bird/frog 类型选择图标
icon := "🐋" // 默认保持鲸鱼
switch birdFrog {
case "bird":
	icon = "🐦"
case "frog":
	icon = "🐸"
}
```

reasoning 分支（`useRoleHeader` case）：

```go
case useRoleHeader:
	Printf("\n%s\n\n", formatChatHeader("💭", disp.String(), "", now))
```

content 分支（`!stream` 内 `useRoleHeader` case）：

```go
case useRoleHeader:
	Printf("\n%s\n\n", formatChatHeader(disp.Icon, disp.String(), "", now))
```

### roles.Display（internal/roles/display.go）

```go
type Display struct {
	Icon  string // emoji 图标
	Role  string // 英文 role 名
	Label string // 中文显示名
}
// String() 返回 "review·代码审查" 形式
```

`roles.DisplayFor(role)` 返回角色 Display；architect = `{Icon: "🏗️", Role: "architect", Label: "软件架构师"}`。

### AIname 注入（chat.go:187 附近，已存在，无需改动）

```go
cfg := ainame.LoadOrAssign(ctx, sessionID)
ctx = context.WithValue(ctx, context.AINameCNKey, cfg.NameCN)
ctx = context.WithValue(ctx, context.AINameEmailKey, cfg.Email)
ctx = context.WithValue(ctx, context.AINameBirdFrogKey, cfg.BirdFrog)
```

## 设计

### 1. formatChatHeader 增加角色标签后缀支持（向后兼容）

把现有函数主体抽为带 roleLabel 参数的实现，原函数保持 4 参签名不变，内部委托：

```go
// formatChatHeader builds a chat header with left-aligned identity
// (icon + name <email> + optional role label) and right-aligned time.
func formatChatHeader(icon, nameCN, email, now string) string {
	return formatChatHeaderWithRole(icon, nameCN, email, "", now)
}

// formatChatHeaderWithRole 在身份后追加角色标签：
//   🐦 玻尔 <bohr@dscli.io> 🏗️ architect·软件架构师 ······ 12:43:10 🕐
// roleLabel 为空时行为与 formatChatHeader 完全一致（所有现有调用点不受影响）。
func formatChatHeaderWithRole(icon, nameCN, email, roleLabel, now string) string {
	left := icon + " " + nameCN
	if email != "" {
		left += " <" + email + ">"
	}
	if roleLabel != "" {
		left += " " + roleLabel
	}
	// …… 以下截断与 padding 逻辑与现状逐字相同 ……
}
```

### 2. PrintContent 角色头分支叠加 Ainame

在 `useRoleHeader` 两处 case 中：

```go
roleLabel := disp.Icon + " " + disp.String() // "🏗️ architect·软件架构师"
hasAIName := nameCN != "" && email != ""
```

reasoning 头：

```go
case useRoleHeader:
	if hasAIName {
		Printf("\n%s\n\n", formatChatHeaderWithRole("💭", nameCN, email, roleLabel, now))
	} else {
		Printf("\n%s\n\n", formatChatHeader("💭", disp.String(), "", now))
	}
```

content 头（注意 icon 用 bird/frog 图标，与现有 `icon` 变量一致）：

```go
case useRoleHeader:
	if hasAIName {
		Printf("\n%s\n\n", formatChatHeaderWithRole(icon, nameCN, email, roleLabel, now))
	} else {
		Printf("\n%s\n\n", formatChatHeader(disp.Icon, disp.String(), "", now))
	}
```

### 3. 为什么这样设计

- **单一渲染路径**：所有对话角色共享 PrintContent，一处改动全角色生效（验收 1）。
- **回退显式**：`hasAIName` 判定后无 Ainame 分支与现状输出逐字节相同（验收 2），纯聊天（role=""）路径完全不动。
- **截断自动**：角色标签作为 left 一部分参与 runewidth 截断，长身份自动 `…`，不会破坏 80 列布局。

## 测试要求

1. `internal/outfmt/output_test.go` 的 `TestFormatChatHeader` 保持通过；新增 `formatChatHeaderWithRole` 用例：
   - 有 roleLabel：输出包含 `architect·软件架构师`（可含图标）、邮箱 `bohr@dscli.io`、无 `<>` 空括号；
   - roleLabel 为空时与 `formatChatHeader` 输出一致；
   - 超宽截断：超长名字 + roleLabel 时行宽不超过 `headerLineWidth`（用 runewidth.StringWidth 断言）且以 `…` 结尾。
2. `chat_test.go` 的 `TestPrintContentRoleHeader` 扩展用例：
   - `role: "architect", cn: "玻尔", email: "bohr@dscli.io", birdFrog: "bird"` → 输出**同时**包含 `玻尔 <bohr@dscli.io>` 与 `architect·软件架构师`；
   - 现有 `review role`（无 cn/email）用例期望不变，继续验证回退；
   - 现有 `no role uses AI name` 用例期望不变（纯聊天路径不受影响）。
3. 不要改动 roles 包、context 包、ainame 逻辑。

## 提交

- 单个 commit，英文 message，如：`feat(outfmt): prepend AI name identity to role headers`
- 提交前运行 `go test ./...` 与 `make fmt-check`，工作树必须干净。
- 本文件（`docs/role-header-ainame.md`）是设计文档，docs/ 目录有文档入仓惯例，请随实现一并 `git add` 提交（不要留在未跟踪状态）。

## 不做的事

- 不改 `PrintUserContent` / `PrintClimeinContent`（🔔/👤 属 Git 用户身份，不是 AI 身份）。
- 不引入新配置项、不改 roles.Display 结构。
- 不迁移 chat.go 的 Ainame 注入逻辑。
