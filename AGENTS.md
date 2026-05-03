# PIXU - Project Guidelines

## Project Overview
Terminal image viewer written in Go. Renders images directly in terminal using various protocols (RGB, Grayscale, 256-color, ASCII, TGP/Kitty/iTerm2, Sixel).

## Build & Run
```bash
go build -o bin/pixu .
go run . --help
```

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
- **macOS**: CGo required for clipboard image support (NSPasteboard). Photoshop puts images as TIFF, not PNG — `clipboard_darwin.m` handles this via NSImage fallback.
- **Linux**: Requires `libx11-dev` for clipboard. Wayland not supported (XWayland only).
- **Windows**: No CGo needed for clipboard.

## Code Conventions
- Comments in English only
- No emojis in code
- Follow existing code style in main.go
- Validate all user inputs (rotate, width, height)
- Use `imaging.AutoOrientation(true)` when loading images
- Clipboard `--paste` tries: binary image → file path → URL → base64 → raw text
- Environment variables override defaults but not explicit CLI flags. Priority: **CLI flag > env variable > default**
- Supported env variables: `PIXU_WIDTH`, `PIXU_HEIGHT`, `PIXU_MODE`, `PIXU_INVERT`, `PIXU_CHAR`, `PIXU_ROTATE`, `PIXU_ASCII_CHARS`, `PIXU_CELL_WIDTH`, `PIXU_CELL_HEIGHT`

## Lint & Verify
```bash
go build -o bin/pixu .
```
