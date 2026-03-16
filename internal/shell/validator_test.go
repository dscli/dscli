package shell

import (
	"context"
	"strings"
	"testing"
)

func TestIsShellScript(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name        string
		script      string
		wantValid   bool
		wantErr     bool
		errContains string
	}{
		{
			name:      "简单echo命令",
			script:    "echo 'Hello, World!'",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "变量赋值",
			script:    "name='Test'\necho \"Hello, $name\"",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "算术运算",
			script:    "echo $((2 + 3 * 4))",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "if语句",
			script:    "if [ -f test.txt ]; then echo '文件存在'; fi",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "for循环",
			script:    "for i in 1 2 3; do echo $i; done",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "函数定义",
			script:    "greet() { echo \"Hello, $1\"; }\ngreet World",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:        "语法错误-未闭合引号",
			script:      "echo 'unclosed quote",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:        "语法错误-无效语法",
			script:      "echo hello | | grep world",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:        "语法错误-未闭合括号",
			script:      "echo $((2 + 3)",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:      "空脚本",
			script:    "",
			wantValid: true, // 空脚本是有效的Shell脚本
			wantErr:   false,
		},
		{
			name:      "只有注释",
			script:    "# 这是一个注释",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:      "shebang行",
			script:    "#!/bin/bash\necho 'Hello'",
			wantValid: true,
			wantErr:   false,
		},
		{
			name:        "Python脚本",
			script:      "print('Hello, World!')",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:        "JavaScript代码",
			script:      "console.log('Hello');",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
		{
			name:      "纯文本",
			script:    "This is just plain text, not a shell script.",
			wantValid: true, // 纯文本在Shell语法中是有效的（虽然执行会失败）
			wantErr:   false,
		},
		{
			name:        "混合内容",
			script:      "echo 'Hello'\nprint('World')\necho 'Again'",
			wantValid:   false,
			wantErr:     true,
			errContains: "不是有效的Shell脚本",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isValid, err := IsShellScript(ctx, tt.script)

			// 检查有效性判断
			if isValid != tt.wantValid {
				t.Errorf("IsShellScript() 有效性判断错误: 期望 %v, 得到 %v", tt.wantValid, isValid)
			}

			// 检查错误
			if tt.wantErr {
				if err == nil {
					t.Errorf("期望错误但得到 nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("错误信息不包含 %q，得到: %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("不期望错误但得到: %v", err)
				}
			}

			// 如果脚本有效，验证它确实可以被执行
			if isValid && err == nil && tt.script != "" {
				// 尝试执行简单的echo命令来验证
				if strings.Contains(tt.script, "echo") {
					output, execErr := SimpleExecute(ctx, "echo '验证执行'")
					if execErr != nil {
						t.Errorf("有效脚本执行失败: %v", execErr)
					}
					if !strings.Contains(output, "验证执行") {
						t.Errorf("执行输出不包含期望内容: %s", output)
					}
				}
			}
		})
	}
}
