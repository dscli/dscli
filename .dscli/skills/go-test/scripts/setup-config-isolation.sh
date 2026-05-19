#!/bin/bash
# go-test setup-config-isolation.sh — 向现有测试文件添加 withConfig helper
# 用法:
#   setup-config-isolation.sh internal/mypackage/my_test.go
#
# 功能：
#   1. 检查文件是否已有 withConfig helper
#   2. 如果没有，在文件开头（package 声明后）插入 withConfig
#   3. 同时添加 "internal/config" import（如不存在）
set -euo pipefail

FILE="${1:-}"
if [ -z "$FILE" ]; then
    echo "用法: setup-config-isolation.sh <test_file.go>"
    exit 1
fi

if [ ! -f "$FILE" ]; then
    echo "❌ 文件不存在: $FILE"
    exit 1
fi

# 检查是否已有 withConfig
if grep -q "func withConfig(t \*testing\.T" "$FILE"; then
    echo "✅ $FILE 已有 withConfig helper，无需操作"
    exit 0
fi

# 检查是否 import config 包
HAS_CONFIG_IMPORT=$(grep -c '"gitcode.com/dscli/dscli/internal/config"' "$FILE" || true)

# 生成 withConfig helper 代码
HELPER=$(cat <<'GOEOF'
// withConfig sets a config value for the duration of the test (or sub-test).
func withConfig(t *testing.T, key, value string) {
	t.Helper()
	old := config.Get(key, "__unset__")
	config.Set(key, value)
	t.Cleanup(func() {
		if old == "__unset__" {
			config.Set(key, "")
		} else {
			config.Set(key, old)
		}
	})
}
GOEOF
)

# 找到第一个 func Test 的位置，在其前面插入 helper
FIRST_TEST_LINE=$(grep -n "^func Test" "$FILE" | head -1 | cut -d: -f1)

if [ -z "$FIRST_TEST_LINE" ]; then
    echo "⚠️  $FILE 中没有 func Test，添加到文件末尾"
    echo "" >> "$FILE"
    echo "$HELPER" >> "$FILE"
else
    # 在第一个 func Test 前插入（前面留空行）
    INSERT_LINE=$((FIRST_TEST_LINE - 1))
    TMP=$(mktemp)
    head -n "$INSERT_LINE" "$FILE" > "$TMP"
    echo "" >> "$TMP"
    echo "$HELPER" >> "$TMP"
    tail -n +$((INSERT_LINE + 1)) "$FILE" >> "$TMP"
    mv "$TMP" "$FILE"
fi

# 添加 import（如不存在）
if [ "$HAS_CONFIG_IMPORT" -eq 0 ]; then
    echo "📝 需要手动添加 import: \"gitcode.com/dscli/dscli/internal/config\""
    echo "   (import 块编辑较复杂，已跳过自动添加)"
fi

echo "✅ 已添加 withConfig helper 到 $FILE"
