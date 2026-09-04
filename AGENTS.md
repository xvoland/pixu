# PIXU - Project Guidelines

## Project Overview
Terminal image viewer written in Go. Renders images directly in terminal using various protocols (RGB, Grayscale, 256-color, ASCII, TGP/Kitty/iTerm2, Sixel).

## Build & Run
```bash
go build -ldflags "-X main.version=$(git describe --tags --abbrev=0 2>/dev/null || echo dev) -X main.buildSource=$(git describe --tags 2>/dev/null || echo local)" -o bin/pixu .
go run . --help
```

The `ldflags` inject the git tag into `--version` (without them `go build` reports
`x.x.x (local)`). For a local single-binary build use `make build-local` (outputs to
`bin/`). For a release cross-build of every platform use `make dist` (outputs to `dist/`).

### Release builds (`make dist`)
- `make dist` — builds all platforms into `dist/`.
- `make dist-windows` / `make dist-linux` / `make dist-darwin` — one OS only.
- `make dist/pixu-<os>-<arch>[.exe]` — a single binary, e.g.
  `make dist/pixu-windows-amd64.exe` or `make dist/pixu-darwin-arm64`.
- Binaries in `dist/` are gitignored; keep the folder marker `dist/.gitkeep`.

Platforms: `windows/amd64`, `windows/arm64`, `linux/amd64`, `linux/arm64`,
`linux/arm`, `darwin/amd64`, `darwin/arm64`.

## QR Code
QR code image (`qr-code.jpg`) is embedded into the binary via `//go:embed` directive in `main.go`. To update the QR code, replace `qr-code.jpg` in the project root and rebuild. No ldflags needed.

## Architecture
- `main.go` — core logic: rendering, CLI, image loading, TGP protocols
- `clipboard_darwin.go` + `clipboard_darwin.m` — macOS clipboard via NSPasteboard (CGo/ObjC), reads NSImage with TIFF fallback
- `clipboard_other.go` — Linux/Windows clipboard via `golang.design/x/clipboard`
- `main_simple.go` — deprecated, excluded from build via `//go:build simple` tag, do not analyze

## Key Dependencies
- `github.com/disintegration/imaging` — image loading, resizing, rotation
- `github.com/spf13/cobra` — CLI framework
- `golang.design/x/clipboard` — clipboard for Linux/Windows (not used on macOS)
- `golang.org/x/term` — terminal size detection
- `github.com/mattn/go-isatty` — TTY detection
- `github.com/mattn/go-sixel` — Sixel graphics protocol for xterm/mlterm

## Platform Notes
- **macOS**: CGo required for clipboard image support (NSPasteboard). Photoshop puts images as TIFF, not PNG — `clipboard_darwin.m` handles this via NSImage fallback. Because of CGo, release cross-builds (`make dist`) set `CGO_ENABLED=1` for darwin targets; darwin binaries must be built on macOS (or with osxcross).
- **Linux**: Clipboard uses `golang.design/x/clipboard` (X11); needs `libx11` at runtime. Wayland is not supported (XWayland only).
- **Windows**: No CGo needed for clipboard.
- **Cross-compilation**: `make dist` builds all platforms in one pass with per-OS CGO (darwin=1, others=0). It works from a macOS host; building darwin binaries from Linux/Windows requires osxcross.

## Code Conventions
- Comments in English only
- No emojis in code
- Follow existing code style in main.go
- Validate all user inputs (rotate, width, height)
- Use `imaging.AutoOrientation(true)` when loading images
- Clipboard `--paste` tries: binary image → file path → URL → base64 → raw text
- Environment variables override defaults but not explicit CLI flags. Priority: **CLI flag > env variable > default**
- Supported env variables: `PIXU_WIDTH`, `PIXU_HEIGHT`, `PIXU_MODE`, `PIXU_INVERT`, `PIXU_CHAR`, `PIXU_ROTATE`, `PIXU_ASCII_CHARS`, `PIXU_CELL_WIDTH`, `PIXU_CELL_HEIGHT`, `PIXU_SCALE`, `PIXU_DITHER`

## Lint & Verify
```bash
go build -ldflags "-X main.version=$(git describe --tags --abbrev=0 2>/dev/null || echo dev) -X main.buildSource=$(git describe --tags 2>/dev/null || echo local)" -o bin/pixu .
```
