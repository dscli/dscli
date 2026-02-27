# Makefile for dscli

BINARY_NAME = dscli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"
GOFLAGS = -trimpath -tags netgo
SOURCE_DIR = .
BUILD_DIR = build

.PHONY: all build clean install test fmt test-coverage coverage coverage-html clean-coverage test-all dev-test watch-test

all: clean build

build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(shell find . -name "*.go")
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(SOURCE_DIR)

install:
	go install $(GOFLAGS) $(LDFLAGS) $(SOURCE_DIR)

clean:
	rm -rf $(BUILD_DIR)

fmt:
	find . -type f -name '*.go' -exec goimports -w {} \; -exec gofumpt -w {} \;

# ==================== 测试相关 ====================

# test: 运行测试（默认）
test: fmt
	@echo "运行测试..."
	@go test -v ./...

# test-coverage: 运行测试并生成覆盖率报告
test-coverage: fmt
	@echo "运行测试并生成覆盖率报告..."
	@echo "=== 覆盖率测试开始 ==="
	@echo ""
	
	# 生成覆盖率文件
	@go test -v -coverprofile=coverage.out ./... 2>&1 | tee test-output.txt
	
	@echo ""
	@echo "=== 覆盖率统计 ==="
	@go tool cover -func=coverage.out
	
	@echo ""
	@echo "=== 生成HTML覆盖率报告 ==="
	@go tool cover -html=coverage.out -o coverage.html
	
	@echo ""
	@echo "✅ 覆盖率报告已生成:"
	@echo "   - coverage.out    : 原始覆盖率数据"
	@echo "   - coverage.html   : HTML可视化报告"
	@echo "   - test-output.txt : 测试详细输出"
	
	# 显示关键覆盖率信息
	@echo ""
	@echo "=== 关键文件覆盖率 ==="
	@go tool cover -func=coverage.out | grep -E "(total:|tools\.go|issue\.go|netrc\.go|db\.go)" | head -15

# coverage: 查看覆盖率报告（如果已生成）
coverage:
	@if [ -f coverage.out ]; then \
		echo "=== 当前覆盖率统计 ==="; \
		go tool cover -func=coverage.out; \
		echo ""; \
		echo "=== 按覆盖率排序 ==="; \
		go tool cover -func=coverage.out | sort -rn -k 3 | head -20; \
	else \
		echo "❌ 覆盖率文件不存在，请先运行: make test-coverage"; \
	fi

# coverage-html: 打开HTML覆盖率报告
coverage-html:
	@if [ -f coverage.html ]; then \
		echo "打开HTML覆盖率报告..."; \
		if command -v xdg-open > /dev/null; then \
			xdg-open coverage.html; \
		elif command -v open > /dev/null; then \
			open coverage.html; \
		else \
			echo "✅ HTML报告已生成: coverage.html"; \
			echo "请用浏览器打开此文件查看详细覆盖率"; \
		fi \
	else \
		echo "❌ HTML覆盖率报告不存在，请先运行: make test-coverage"; \
	fi

# clean-coverage: 清理覆盖率文件
clean-coverage:
	@echo "清理覆盖率文件..."
	@rm -f coverage.out coverage.html test-output.txt
	@echo "✅ 覆盖率文件已清理"

# test-all: 运行所有测试（包括覆盖率）
test-all: test-coverage coverage

# ==================== 开发辅助 ====================

# dev-test: 快速开发测试（不重新格式化）
dev-test:
	@echo "快速运行测试（跳过格式化）..."
	@go test -v ./...

# watch-test: 监控文件变化并运行测试（需要安装fswatch或inotifywait）
watch-test:
	@echo "监控文件变化并运行测试..."
	@echo "需要安装文件监控工具，例如:"
	@echo "  Linux: sudo apt-get install inotify-tools"
	@echo "  macOS: brew install fswatch"
	@echo ""
	@if command -v fswatch > /dev/null; then \
		echo "使用 fswatch 监控 .go 文件变化..."; \
		fswatch -o *.go **/*.go | xargs -n1 -I{} make dev-test; \
	elif command -v inotifywait > /dev/null; then \
		echo "使用 inotifywait 监控 .go 文件变化..."; \
		while true; do \
			inotifywait -e modify -r --include='\.go$$' .; \
			make dev-test; \
		done; \
	else \
		echo "❌ 未找到文件监控工具，请安装 fswatch 或 inotify-tools"; \
	fi
