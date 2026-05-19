#!/bin/bash
# go-test run.sh — 项目标准测试运行器
# 用法:
#   run.sh                       默认：全部测试，-race, -count=1
#   run.sh --short               跳过慢测试（集成/e2e）
#   run.sh --verbose             详细输出
#   run.sh internal/config/...   只跑指定包
#   run.sh --short --verbose internal/lp/...
set -euo pipefail

SHORT=""
VERBOSE=""
EXTRA_FLAGS=""
PACKAGES=()

while [[ $# -gt 0 ]]; do
    case "$1" in
        --short)  SHORT="-short" ;;
        --verbose|-v) VERBOSE="-v" ;;
        --race=*) EXTRA_FLAGS="$EXTRA_FLAGS $1" ;;
        *) PACKAGES+=("$1") ;;
    esac
    shift
done

if [ ${#PACKAGES[@]} -eq 0 ]; then
    PACKAGES=("./...")
fi

FLAGS="-race -count=1 -vet=all $SHORT $VERBOSE $EXTRA_FLAGS"

echo "🏃 go test $FLAGS ${PACKAGES[*]}"
echo ""

# 运行测试。用 go test 原样输出，最后汇总。
set +e
go test $FLAGS "${PACKAGES[@]}" 2>&1 | tee /tmp/go-test-output.txt
EXIT_CODE=${PIPESTATUS[0]}
set -e

echo ""
echo "════════════════════════════════════════"
# 汇总统计
PASSED=$(grep -c '^ok' /tmp/go-test-output.txt || true)
FAILED=$(grep -c '^FAIL' /tmp/go-test-output.txt || true)
SKIPPED=$(grep -c '\[skipped\]' /tmp/go-test-output.txt || true)
echo "✅ passed:  $PASSED"
echo "❌ failed:  $FAILED"
if [ "$SKIPPED" -gt 0 ]; then
    echo "⏭️  skipped: $SKIPPED"
    echo ""
    echo "跳过的测试（缺少外部依赖）："
    grep '\[skipped\]' /tmp/go-test-output.txt | sed 's/^/  /'
fi
echo "════════════════════════════════════════"

rm -f /tmp/go-test-output.txt
exit $EXIT_CODE
