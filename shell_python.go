package main

import (
	"context"
	"strings"
	"time"
)

func init() {
	// 注册python工具
	RegisterTool(ToolDef{
		Name: "python",
		Description: `在项目根目录执行Python脚本。
脚本通过标准输入传递，避免命令行长度限制。

输出格式：
- 成功时：返回包含执行结果和执行统计的格式化文本
- 失败时：返回包含错误信息、输出内容和执行统计的格式化文本

示例：
1. Python脚本：print("Hello")
2. 数据处理：import json; print(json.dumps({"key": "value"}))
3. 文件操作：with open("file.txt", "r") as f: print(f.read())

注意：谨慎使用，避免破坏性操作。确保脚本在项目目录内执行。`,
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `要执行的Python脚本内容。
支持shebang指定解释器（如#!/usr/bin/env python, #!/usr/bin/env python3）。
脚本执行结果会以格式化文本返回，包含执行统计信息。

示例：
1. Python脚本：print("Hello")
2. 数据处理：import json; print(json.dumps({"key": "value"}))
3. 文件操作：with open("file.txt", "r") as f: print(f.read())
`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "system",
		Timeout:  60 * time.Second, // 设置60秒超时
		Handler:  handlePython,
	})
}

// handlePython 执行Python脚本
func handlePython(ctx context.Context, args map[string]string) (out string, err error) {
	script, ok := args["script"]
	if !ok {
		script = ""
	}

	// 检查危险命令（Python脚本也可能包含shell命令）
	if err := checkDangerousCommands(script); err != nil {
		return "", err
	}

	// 如果没有shebang，添加默认的python shebang
	if !strings.HasPrefix(strings.TrimSpace(script), "#!") {
		script = "#!/usr/bin/env python3\n" + script
	}

	out, err = runShell(ctx, script)
	return
}
