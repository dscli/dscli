package main

import (
	"context"
	"fmt"
)

func init() {
	RegisterTool(ToolDef{
		Name:        "sqlite",
		Description: "执行SQLite数据库查询和操作。脚本内容为SQL语句。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"script": map[string]any{
					"type": "string",
					"description": `sqlite SQL脚本内容。
例如：
1. .schema messages      Show the CREATE statements matching PATTERN
2. select id, role from messages where id > 1000 order by created_at desc;`,
				},
			},
			"required":             []string{"script"},
			"additionalProperties": false,
		},
		Category: "database",
		Handler:  handleSqlite,
	})
}

// handleSqlite 执行SQLite数据库查询和操作
func handleSqlite(ctx context.Context, args ToolArgs) (string, error) {
	script := ToolArgsValue(args, "script", "")
	if script == "" {
		return "", fmt.Errorf("sql script can not be empty")
	}
	// 构建完整的shebang脚本
	fullScript := fmt.Sprintf("#!/usr/bin/env sqlite3 %s\n%s", GetDBPath(), script)

	// 使用现有的runBash执行
	return runShell(ctx, fullScript)
}
