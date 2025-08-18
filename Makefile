# Go cross-compilation Makefile with ARM support (static binaries)

APP_NAME := img2ascii
SRC := main.go
BIN_DIR := bin

# Platforms: OS/ARCH
PLATFORMS := windows/amd64 linux/amd64 darwin/amd64 linux/arm64 linux/arm

.PHONY: all clean build

all: clean build

build:
	@mkdir -p $(BIN_DIR)
	@for platform in $(PLATFORMS); do \
		OS=$${platform%/*}; \
		ARCH=$${platform#*/}; \
		OUT=$(BIN_DIR)/$(APP_NAME)-$${OS}-$${ARCH}; \
		if [ "$${OS}" = "windows" ]; then OUT=$${OUT}.exe; fi; \
		echo "Building $$OUT..."; \
		CGO_ENABLED=0 GOOS=$${OS} GOARCH=$${ARCH} go build -o $$OUT $(SRC); \
	done

clean:
	@rm -rf $(BIN_DIR)
