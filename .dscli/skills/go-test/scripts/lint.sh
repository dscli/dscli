#!/bin/bash
# go-test lint.sh — 检查测试文件中的项目特有反模式
# 用法:
#   lint.sh                    检查全部
#   lint.sh internal/lp/...     检查指定包
set -euo pipefail

TARGET="${1:-.}"
ISSUES=0

echo "🔍 检查 $TARGET 中的测试反模式…"
echo ""

# ── 1. config 隔离 ──
echo "── 1. config 隔离 ──"
TMPFILE=$(mktemp)
for f in $(find "$TARGET" -name "*_test.go" -type f); do
    awk '
    /^func withConfig\(t \*testing\.T/ { in_withconfig = 1 }
    in_withconfig && /^}/            { in_withconfig = 0; next }
    in_withconfig                     { next }
    /config\.Set\(/                   { print FILENAME ":" NR ": " $0 }
    ' "$f"
done > "$TMPFILE"
if [ -s "$TMPFILE" ]; then
    echo "⚠️  直接调用 config.Set 未通过 withConfig 隔离："
    cat "$TMPFILE" | sed 's/^/    /'
    ISSUES=$((ISSUES + $(wc -l < "$TMPFILE")))
else
    echo "✅ 通过"
fi
rm -f "$TMPFILE"

# ── 2. 函数变量隔离 ──
echo ""
echo "── 2. 函数变量隔离 ──"
TMPFILE=$(mktemp)
for f in $(find "$TARGET" -name "*_test.go" -type f); do
    if grep -qP '^\t[a-zA-Z_][a-zA-Z0-9_]* = func\(' "$f"; then
        if ! grep -qE 'defer (restoreFuncVars|func\(\) \{[^}]*= old)' "$f"; then
            echo "$f: has package-level func var assignment without restore" >> "$TMPFILE"
        fi
    fi
done
if [ -s "$TMPFILE" ]; then
    echo "⚠️  包级函数变量赋值未找到 restore 机制："
    cat "$TMPFILE" | sed 's/^/    /'
    ISSUES=$((ISSUES + $(wc -l < "$TMPFILE")))
else
    echo "✅ 通过"
fi
rm -f "$TMPFILE"

# ── 3. 空测试文件 ──
echo ""
echo "── 3. 空测试文件 ──"
TMPFILE=$(mktemp)
for f in $(find "$TARGET" -name "*_test.go" -type f); do
    count=$(grep -c "^func Test" "$f" 2>/dev/null) || count=0
    if [ "$count" -eq 0 ] 2>/dev/null; then
        echo "$f" >> "$TMPFILE"
    fi
done
if [ -s "$TMPFILE" ]; then
    echo "⚠️  无测试函数的 _test.go 文件（可能仅包含 helper/benchmark）："
    cat "$TMPFILE" | sed 's/^/    /'
    ISSUES=$((ISSUES + $(wc -l < "$TMPFILE")))
else
    echo "✅ 通过"
fi
rm -f "$TMPFILE"

# ── 4. t.Helper 缺失 ──
# 只检查非 Test/Benchmark/Fuzz 的函数（helper 函数），排除 TestMain
echo ""
echo "── 4. t.Helper 缺失 ──"
TMPFILE=$(mktemp)
for f in $(find "$TARGET" -name "*_test.go" -type f); do
    awk '
    /^func (Test|Fuzz|Benchmark)[A-Z_].*\(t \*testing\.T/ { next }
    /^func .*\(t \*testing\.T/ {
        fn = $0; fnline = NR
        found = 0
        for (i = 1; i <= 5; i++) {
            ret = getline
            if (ret <= 0) break
            if ($0 ~ /t\.Helper\(\)/) { found = 1; break }
            if ($0 ~ /^}/) break
        }
        if (!found && NR > fnline) {
            print FILENAME ":" fnline ": " fn
        }
    }
    ' "$f"
done > "$TMPFILE"
if [ -s "$TMPFILE" ]; then
    echo "⚠️  缺少 t.Helper() 的测试辅助函数："
    cat "$TMPFILE" | sed 's/^/    /'
    ISSUES=$((ISSUES + $(wc -l < "$TMPFILE")))
else
    echo "✅ 通过"
fi
rm -f "$TMPFILE"

# ── 总结 ──
echo ""
echo "══════════════════════════════"
if [ "$ISSUES" -eq 0 ]; then
    echo "✅ 未发现反模式"
else
    echo "⚠️  发现 $ISSUES 个问题"
fi
echo "══════════════════════════════"
exit 0
