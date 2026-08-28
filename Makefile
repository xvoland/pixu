# Go cross-compilation Makefile with ARM support (static binaries)

APP_NAME := pixu
SRC := main.go
BIN_DIR := bin

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
BUILD_SOURCE := $(shell git describe --tags 2>/dev/null || echo "local")

QR_CODE_BASE64 := $(shell base64 -i qr-code.jpg | tr -d '\n')

PLATFORMS := windows/amd64 linux/amd64 darwin/amd64 linux/arm64 linux/arm

LDFLAGS := -X main.version=$(VERSION) -X main.buildSource=$(BUILD_SOURCE)

.PHONY: all clean build build-local qr

all: clean build

build-local:
	@mkdir -p $(BIN_DIR)
	@echo "Building $(APP_NAME) $(VERSION)..."
	@CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) .

build:
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUT=$(BIN_DIR)/$(APP_NAME)-$${OS}-$${ARCH}; \
		if [ "$${OS}" = "windows" ]; then OUT=$${OUT}.exe; fi; \
		echo "Building $$OUT ($(VERSION))..."; \
		CGO_ENABLED=0 GOOS=$${OS} GOARCH=$${ARCH} go build -ldflags "$(LDFLAGS)" -o $$OUT .; \
	done

clean:
	@rm -rf $(BIN_DIR)

.PHONY: qr

qr:
	@$(BIN_DIR)/$(APP_NAME) --qr
