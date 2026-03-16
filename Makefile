# Makefile for dscli

BINARY_NAME = dscli

# 版本信息：优先使用git标签，如果没有标签则使用git提交哈希
GIT_TAG = $(shell git describe --tags 2>/dev/null || echo "")
GIT_COMMIT = $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_DATE = $(shell date -u +'%Y-%m-%dT%H:%M:%SZ')

# 如果存在git标签，使用标签作为版本号，否则使用提交哈希
ifeq ($(GIT_TAG),)
	VERSION ?= $(GIT_COMMIT)
else
	VERSION ?= $(GIT_TAG)
endif

# 如果VERSION为空或为unknown，使用开发版本
ifeq ($(VERSION),)
	VERSION = dev
else ifeq ($(VERSION),unknown)
	VERSION = dev
endif

LDFLAGS = -ldflags "-X main.Version=$(VERSION) -X main.Build=$(BUILD_DATE)-$(GIT_COMMIT)"
GOFLAGS = -trimpath -tags netgo
SOURCE_DIR = .
BUILD_DIR = build

.PHONY: all build clean install test fmt test-coverage coverage coverage-html clean-coverage test-all dev-test watch-test release

all: clean build

build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(shell find . -name "*.go")
	@mkdir -p $(BUILD_DIR)
	@echo "构建 dscli $(VERSION) (commit: $(GIT_COMMIT), date: $(BUILD_DATE))"
	CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(SOURCE_DIR)

install:
	@echo "安装 dscli $(VERSION)"
	go install $(GOFLAGS) $(LDFLAGS) $(SOURCE_DIR)

clean:
	rm -rf $(BUILD_DIR)

fmt:
	@echo "运行 goimports 和 gofumpt 格式化..."
	@find . -type f -name '*.go' -exec goimports -w {} \; -exec gofumpt -w {} \;
	@echo "运行 modernize 现代化工具..."
	@modernize -any -fix ./...

fmt-check:
	@echo "检查代码格式（不修改文件）..."
	@echo "检查 goimports..."
	@find . -type f -name '*.go' -exec goimports -d {} \; | grep -v "^$" || true
	@echo "检查 gofumpt..."
	@find . -type f -name '*.go' -exec gofumpt -d {} \; | grep -v "^$" || true
	@echo "检查 modernize..."
	@echo "注意：modernize 不支持 -check 参数，使用 -any -fix 但只显示不修改"
	@modernize -any ./... || true

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

# ==================== 发布相关 ====================

# release: 构建发布版本
release: clean
	@echo "构建 dscli v$(VERSION) 发布版本..."
	@echo "构建时间: $(BUILD_DATE)"
	@echo "Git提交: $(GIT_COMMIT)"
	@echo ""
	# 构建Linux版本
	@echo "=== 构建Linux版本 ==="
	GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(SOURCE_DIR)
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-arm64 $(SOURCE_DIR)
	# 构建macOS版本
	@echo "=== 构建macOS版本 ==="
	GOOS=darwin GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(SOURCE_DIR)
	GOOS=darwin GOARCH=arm64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(SOURCE_DIR)
	# 构建Windows版本
	@echo "=== 构建Windows版本 ==="
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(SOURCE_DIR)
	@echo ""
	@echo "✅ 发布版本构建完成！"
	@echo "输出目录: $(BUILD_DIR)/"
	@ls -lh $(BUILD_DIR)/*
	@echo ""
	@echo "版本信息验证:"
	@for binary in $(BUILD_DIR)/*; do \
		if [ -x $$binary ] || [[ $$binary == *.exe ]]; then \
			echo -n "$$binary: "; \
			if [[ $$binary == *.exe ]]; then \
				wine $$binary version 2>/dev/null | head -1 || echo "无法执行"; \
			else \
				$$binary version 2>/dev/null | head -1 || echo "无法执行"; \
			fi; \
		fi; \
	done

# release-info: 显示发布信息
release-info:
	@echo "=== dscli v$(VERSION) 发布信息 ==="
	@echo "版本号: $(VERSION)"
	@echo "构建时间: $(BUILD_DATE)"
	@echo "Git提交: $(GIT_COMMIT)"
	@echo "支持的平台:"
	@echo "  - Linux (amd64, arm64)"
	@echo "  - macOS (amd64, arm64)"
	@echo "  - Windows (amd64)"
	@echo ""
	@echo "构建命令: make release"
	@echo "安装命令: make install 或 go install gitcode.com/dscli/dscli@v$(VERSION)"

# version-info: 显示版本信息
version-info:
	@echo "=== 版本信息 ==="
	@echo "Git标签: $(GIT_TAG)"
	@echo "Git提交: $(GIT_COMMIT)"
	@echo "构建时间: $(BUILD_DATE)"
	@echo "最终版本: $(VERSION)"
	@echo ""
	@echo "构建标志:"
	@echo "  -X main.Version=$(VERSION)"
	@echo "  -X main.Build=$(BUILD_DATE)-$(GIT_COMMIT)"
