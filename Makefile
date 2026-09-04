# Go cross-compilation Makefile with per-platform targets.
#
# `make dist` builds every platform into ./dist (the canonical release output).
# `make dist-windows` / `make dist-linux` / `make dist-darwin` build one OS.
# A single binary is built with `make build-local` into ./bin for local dev.
#
# darwin targets require CGO_ENABLED=1 and must be built on macOS (or with
# osxcross); all other targets are pure-Go (CGO_ENABLED=0). Cross-building
# darwin from Linux/Windows is not supported by this Makefile.

APP_NAME := pixu
SRC := main.go

BIN_DIR := bin    # local single-binary dev build
DIST_DIR := dist  # multi-platform release builds

VERSION := $(shell git describe --tags --abbrev=0 2>/dev/null || echo "dev")
BUILD_SOURCE := $(shell git describe --tags 2>/dev/null || echo "local")

LDFLAGS := -X main.version=$(VERSION) -X main.buildSource=$(BUILD_SOURCE)

# os/arch pairs to build for distribution.
PLATFORMS := windows/amd64 windows/arm64 \
             linux/amd64 linux/arm64 linux/arm \
             darwin/amd64 darwin/arm64

# dist/pixu-<os>-<arch>[.exe] for every platform above.
DIST_BINS := $(foreach p,$(PLATFORMS),\
  dist/$(APP_NAME)-$(word 1,$(subst /, ,$(p)))-$(word 2,$(subst /, ,$(p)))$(if $(filter windows,$(word 1,$(subst /, ,$(p)))),.exe,))

.PHONY: all clean dist build-local qr \
        dist-windows dist-linux dist-darwin

all: clean dist

# Local development build (single binary, current platform).
build-local:
	@mkdir -p $(BIN_DIR)
	@echo "Building $(APP_NAME) $(VERSION) (local)..."
	@CGO_ENABLED=1 go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(APP_NAME) .

# Build every platform into ./dist.
dist: $(DIST_BINS)

# Build one OS (all of its architectures).
dist-windows: $(filter dist/$(APP_NAME)-windows-%,$(DIST_BINS))
dist-linux:   $(filter dist/$(APP_NAME)-linux-%,$(DIST_BINS))
dist-darwin:  $(filter dist/$(APP_NAME)-darwin-%,$(DIST_BINS))

# Per-platform rule. Invoke a single binary directly, e.g.:
#   make dist/pixu-windows-amd64.exe
#   make dist/pixu-darwin-arm64
define build_one
dist/$(APP_NAME)-$(1)-$(2)$(if $(filter windows,$(1)),.exe,): $(SRC)
	@mkdir -p $(DIST_DIR)
	@echo "Building $$@ ($(VERSION), CGO_ENABLED=$(if $(filter darwin,$(1)),1,0))..."
	CGO_ENABLED=$(if $(filter darwin,$(1)),1,0) GOOS=$(1) GOARCH=$(2) go build -ldflags "$(LDFLAGS)" -o $$@ .
endef
$(foreach p,$(PLATFORMS),$(eval $(call build_one,$(word 1,$(subst /, ,$(p))),$(word 2,$(subst /, ,$(p))))))

clean:
	@rm -rf $(BIN_DIR) $(DIST_DIR)

qr:
	@$(BIN_DIR)/$(APP_NAME) --qr
