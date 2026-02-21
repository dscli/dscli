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
	find . -type f -name '*.go' -exec goimports -w {} \; -exec gofumpt -w {} \;

test:
	go test ./...
