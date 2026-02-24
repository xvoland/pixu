# Go cross-compilation Makefile with ARM support (static binaries)

APP_NAME := pixu
SRC := main.go
BIN_DIR := bin

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
BUILD_SOURCE := $(shell git describe --tags 2>/dev/null || echo "local")

QR_CODE_BASE64 := $(shell base64 -i qr-code.jpg | tr -d '\n')

PLATFORMS := windows/amd64 linux/amd64 darwin/amd64 linux/arm64 linux/arm

LDFLAGS := -X main.version=$(VERSION) -X main.buildSource=$(BUILD_SOURCE) -X main.qrCodeBase64=$(QR_CODE_BASE64)

.PHONY: all clean build build-local qr

all: clean build

build-local:
	@echo "Building $(APP_NAME) $(VERSION)..."
	@go build -ldflags "$(LDFLAGS)" -o $(APP_NAME) $(SRC)

build:
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUT=$(BIN_DIR)/$(APP_NAME)-$${OS}-$${ARCH}; \
		if [ "$${OS}" = "windows" ]; then OUT=$${OUT}.exe; fi; \
		echo "Building $$OUT ($(VERSION))..."; \
		CGO_ENABLED=0 GOOS=$${OS} GOARCH=$${ARCH} go build -ldflags "$(LDFLAGS)" -o $$OUT $(SRC); \
	done

clean:
	@rm -rf $(BIN_DIR)

.PHONY: qr

qr:
	@./pixu --qr
