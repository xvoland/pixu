# PIXU - Terminal Image Viewer

<p align="center">
  <a href="https://dotoca.net/pixu">
    <img src="https://dotoca.net/pixu/logo.png" alt="PIXU logo" width="200" height="200">
  </a>
</p>

<p align="center">
  <a href="https://github.com/xvoland/pixu/actions">
    <img src="https://img.shields.io/github/actions/workflow/status/xvoland/pixu/build.yml" alt="Build">
  </a>
  <a href="https://goreportcard.com/report/github.com/xvoland/pixu">
    <img src="https://goreportcard.com/badge/github.com/xvoland/pixu" alt="Go Report Card">
  </a>
  <a href="https://github.com/xvoland/pixu/releases">
    <img src="https://img.shields.io/github/v/release/xvoland/pixu" alt="Release">
  </a>
  <a href="https://opensource.org/licenses/Apache-2.0">
    <img src="https://img.shields.io/badge/License-Apache%202.0-blue.svg" alt="License">
  </a>
</p>

> PIXU is a powerful terminal image viewer that renders images directly in your terminal using various protocols and formats.

## Features

- **Multiple rendering modes**: RGB, Grayscale, 256-color, ASCII art, and TGP (Terminal Graphics Protocol)
- **iTerm2 & Kitty support**: Display images inline in modern terminal emulators
- **Interactive mode**: Browse through images with keyboard navigation
- **Clipboard support**: Read images directly from clipboard
- **Pipeline support**: Use with stdin/stdout for shell pipelines
- **Cross-platform**: Works on Linux, macOS, and Windows
- **Color manipulation**: Invert colors, apply dithering

## Installation

### From Source

```bash
go install github.com/xvoland/pixu@latest
```

### From Release

```bash
# Linux/macOS
curl -sL https://github.com/xvoland/pixu/releases/latest/download/pixu-linux-amd64 -o pixu
chmod +x pixu

# macOS (Apple Silicon)
curl -sL https://github.com/xvoland/pixu/releases/latest/download/pixu-darwin-arm64 -o pixu
chmod +x pixu

# Windows
curl -sL https://github.com/xvoland/pixu/releases/latest/download/pixu-windows-amd64.exe -o pixu.exe
```

### Build from Source

```bash
git clone https://github.com/xvoland/pixu.git
cd pixu
make build-local
```

## Quick Start

```bash
# View an image (default: RGB mode)
pixu image.png

# View in ASCII art
pixu image.png --mode ascii

# View in iTerm2/Kitty (TGP mode)
pixu image.png --mode tgp

# Fit to terminal size
pixu image.png --fit
```

## Usage

### Basic Examples

```bash
# Display image in RGB mode (default)
pixu photo.jpg

# Display image in ASCII art
pixu photo.jpg --mode ascii

# Display image in 256-color mode
pixu photo.jpg --mode 256

# Display image in grayscale
pixu photo.jpg --mode grayscale

# Display using Kitty/iTerm2 graphics protocol
pixu photo.jpg --mode tgp
```

### Resizing

```bash
# Set width (height calculated automatically)
pixu image.png --width 80

# Set height (width calculated automatically)
pixu image.png --height 40

# Set both width and height
pixu image.png --width 100 --height 50

# Fit to terminal size
pixu image.png --fit

# With TGP mode
pixu image.png --mode tgp --width 800 --height 600
```

### Image Manipulation

```bash
# Invert colors
pixu image.png --invert

# Rotate 90 degrees
pixu image.png --rotate 90

# Rotate 180 degrees
pixu image.png --rotate 180

# Rotate 270 degrees
pixu image.png --rotate 270

# Apply Floyd-Steinberg dithering
pixu image.png --dither

# Combine options
pixu image.png --rotate 90 --invert --width 60
```

### Custom Characters

```bash
# Use custom block character (default: ▀)
pixu image.png --char "█"

# Use custom ASCII character set
pixu image.png --mode ascii --char "@#%*+=-:. "

# Use different character for RGB mode
pixu image.png --char "▓"
```

### Input Sources

```bash
# From file (default)
pixu image.png

# From stdin
cat image.png | pixu -
pixu - < image.png

# From clipboard
pixu --paste
pixu -p

# From specific input file
pixu --input image.jpg
```

### Output

```bash
# Save to file
pixu image.png --output output.txt

# Save ASCII art to file
pixu image.png --mode ascii --output art.txt

# Pipe to file
pixu image.png > output.txt
```

### Interactive Mode

```bash
# Start interactive viewer
pixu image.png --interactive
pixu image.png -I

# Browse directory of images
pixu ./photos --interactive
pixu ./photos -I

# Interactive mode with TGP
pixu image.png --mode tgp --interactive
```

In interactive mode, use:
- `←` / `→` - Previous/Next image
- `n` / `p` or `Ctrl+N` / `Ctrl+P` - Next/Previous
- `q` / `ESC` / `Ctrl+C` - Quit

### Other Options

```bash
# Show version
pixu --version

# Show QR code for donation
pixu --qr

# Environment variables
export PIXU_WIDTH=80
export PIXU_HEIGHT=40
export PIXU_MODE=ascii
export PIXU_INVERT=true
export PIXU_CHAR="█"
export PIXU_ROTATE=90
export PIXU_ASCII_CHARS="@#%*+=-:. "

# Custom terminal cell size
export PIXU_CELL_WIDTH=10
export PIXU_CELL_HEIGHT=20
```

## Mode Details

| Mode | Description | Terminal Support |
|------|-------------|------------------|
| `rgb` | True color (24-bit) with block characters | All terminals |
| `grayscale` | 256 shades of gray | All terminals |
| `256` | 256-color mode | Most terminals |
| `ascii` | Plain ASCII art | All terminals |
| `tgp` | Terminal Graphics Protocol (iTerm2/Kitty) | iTerm2, Kitty, Ghostty, WezTerm |

## Requirements

- Go 1.25 or later (for building from source)
- Terminal with true color support (for RGB mode)
- iTerm2, Kitty, Ghostty, or WezTerm (for TGP mode)

## Configuration

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `PIXU_WIDTH` | Default width | - |
| `PIXU_HEIGHT` | Default height | - |
| `PIXU_MODE` | Default mode | `rgb` |
| `PIXU_INVERT` | Invert colors | `false` |
| `PIXU_CHAR` | Block character | `▀` |
| `PIXU_ROTATE` | Rotation | `0` |
| `PIXU_ASCII_CHARS` | ASCII character set | `@#%*+=-:. ` |
| `PIXU_CELL_WIDTH` | Terminal cell width (px) | `10` |
| `PIXU_CELL_HEIGHT` | Terminal cell height (px) | `20` |

## Use Cases

### Pipeline Examples

```bash
# Convert image and copy to clipboard
pixu image.png --mode ascii | tee art.txt | pbcopy

# Generate ASCII art for documentation
convert photo.jpg -resize 80x png:- | pixu --mode ascii

# Quick preview from URL
curl -s image.png | pixu -
```

### Shell Aliases

```bash
# Add to ~/.bashrc or ~/.zshrc
alias p='pixu --mode ascii --width 80'
alias pxl='pixu --mode tgp --fit'
alias pxs='pixu --interactive'
```

### Scripts

```bash
#!/bin/bash
# View all images in directory
for img in *.jpg *.png *.gif; do
  [ -f "$img" ] && pixu "$img" --width 60
done
```

## Troubleshooting

### Image not displaying correctly

- Use `--mode tgp` for iTerm2, Kitty, Ghostty, or WezTerm
- Use `--mode 256` for older terminals
- Check terminal color support: `printf '\e[48;2;255;0;0mTEST\e[0m\n'`

### Wrong aspect ratio

- Use `--fit` to automatically fit terminal
- Adjust `--width` and `--height`
- Set custom cell size: `PIXU_CELL_WIDTH=12 PIXU_CELL_HEIGHT=24`

### Performance

- Use smaller `--width` for faster rendering
- Disable dithering (`--dither` can be slow on large images)

## License

Licensed under the Apache License, Version 2.0. See [LICENSE](LICENSE) for details.

## Author

Copyright © 2026, [Vitalii Tereshchuk](https://dotoca.net)

- Homepage: https://dotoca.net/pixu
- GitHub: https://github.com/xvoland/pixu
- Donation: https://paypal.me/xvoland