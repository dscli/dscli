package roles

// Display 是角色在终端输出中的显示元数据。
type Display struct {
	Icon  string // emoji 图标
	Name  string // 英文 role 名（dev/expert/review/test/architect）
	Label string // 中文显示名
}

// String 返回 "review·代码审查" 形式；Name 为空时返回 ""。
func (d Display) String() string {
	if d.Name == "" {
		return ""
	}
	return d.Name + "·" + d.Label
}

var (
	devDisplay       = Display{Icon: "💻", Name: "dev", Label: "开发助手"}
	expertDisplay    = Display{Icon: "🧠", Name: "expert", Label: "领域专家"}
	reviewDisplay    = Display{Icon: "🔍", Name: "review", Label: "代码审查"}
	testDisplay      = Display{Icon: "🧪", Name: "test", Label: "QA 工程师"}
	architectDisplay = Display{Icon: "🏗️", Name: "architect", Label: "软件架构师"}
)

// DisplayFor 返回角色的显示元数据。"" 返回零值 Display{}（调用方应走
// 默认 AI 名路径）；未知角色 fallback dev（与 DefaultFor 的未知角色策略一致）。
func DisplayFor(role string) Display {
	switch role {
	case "dev":
		return devDisplay
	case "expert":
		return expertDisplay
	case "review":
		return reviewDisplay
	case "test":
		return testDisplay
	case "architect":
		return architectDisplay
	case "":
		return Display{}
	default:
		return devDisplay
	}
}

// DisplayName 返回消息/提示中的角色称谓："" → "专家"（纯聊天通用称谓）；
// 未知角色回退 dev 的组合名。
func DisplayName(role string) string {
	if role == "" {
		return "专家"
	}
	if s := DisplayFor(role).String(); s != "" {
		return s
	}
	return "专家"
}
