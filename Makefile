# Makefile for dscli

BINARY_NAME = dscli
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS = -ldflags "-X main.version=$(VERSION)"
GOFLAGS = -trimpath
SOURCE_DIR = .
BUILD_DIR = build

.PHONY: all build clean install test fmt

all: clean build

build: $(BUILD_DIR)/$(BINARY_NAME)

$(BUILD_DIR)/$(BINARY_NAME): $(shell find . -name "*.go")
	@mkdir -p $(BUILD_DIR)
	go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) $(SOURCE_DIR)

install:
	go install $(GOFLAGS) $(LDFLAGS) $(SOURCE_DIR)

clean:
	rm -rf $(BUILD_DIR)

fmt:
	find . -type f -name '*.go' -exec goimports -w {} \;
	find . -type f -name '*.go' -exec gofumpt -w {} \;

test:
	go test ./...

# 交叉编译示例（可选）
build-linux:
	GOOS=linux GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 $(SOURCE_DIR)

build-windows:
	GOOS=windows GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe $(SOURCE_DIR)

build-macos:
	GOOS=darwin GOARCH=amd64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 $(SOURCE_DIR)
	GOOS=darwin GOARCH=arm64 go build $(GOFLAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 $(SOURCE_DIR)
