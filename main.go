/*
   Copyright © 2026, Vitalii Tereshchuk | DOTOCA.NET All rights reserved.
   Homepage: https://dotoca.net/pixu

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

   http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.

*/

package main

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/mattn/go-isatty"
	"github.com/mattn/go-sixel"
	"github.com/spf13/cobra"
	_ "golang.org/x/image/webp"
	"golang.org/x/term"
)

const defaultVersion = "x.x.x"

// versionLine returns the version string for display. The build source is shown
// in parentheses only when it differs from the version (e.g. a dev or dirty
// build), so a clean release tag reads simply "pixu v1.0.0".
// printInfo prints the project banner shown for --version, the `version`
// subcommand, and the no-argument case.
func printInfo() {
	year := time.Now().Year()
	fmt.Printf("PIXU, %s | https://dotoca.net/pixu\n", version)
	fmt.Printf("(c) %d, Vitalii Tereshchuk | xVoLAnD. All rights reserved.\n\n", year)
}

const (
	cellWidthPxDefault  = 10 // approximate terminal cell width in pixels
	cellHeightPxDefault = 20 // approximate terminal cell height in pixels

	statusLinesTGP         = 4 // status lines for TGP mode
	statusLinesTerminal    = 4 // status lines for terminal rendering (fit mode)
	statusLinesInteractive = 3 // status lines for interactive mode (excluding header)

	maxWidth  = 10000 // maximum allowed width in characters
	maxHeight = 10000 // maximum allowed height in characters
)

// getCellSize returns terminal cell size in pixels, with env override
// getCellSize returns the terminal cell size in pixels. It is used to convert
// character columns/rows into pixel dimensions for the tgp and sixel protocols,
// where width/height are expressed in pixels rather than character cells.
func getCellSize() (int, int) {
	return cellWidth, cellHeight
}

var qrCodeText = "Scan to support PIXU!\nhttps://paypal.me/xvoland\n"

var buildSource = "local"

// httpClient is used for fetching images by URL (e.g. from clipboard).
// A timeout prevents the tool from hanging indefinitely on an unresponsive host.
var httpClient = &http.Client{Timeout: 15 * time.Second}

//go:embed qr-code.jpg
var qrCodeData []byte

var (
	width, height, rotate int
	mode                  string
	invert                bool
	char                  string
	version               = defaultVersion
	showVersion           bool
	fit                   string // --fit axis: "H" (by height, default) or "W" (by width)
	dither                bool
	interactive           bool
	qr                    bool
	input                 string
	paste                 bool
	output                string
	scale                 = 1.0

	// asciiChars is the ramp used in ascii mode (overridable via PIXU_ASCII_CHARS).
	asciiChars = "@#%*+=-:. "
	// cellWidth/cellHeight are terminal cell dimensions in pixels (overridable via env).
	cellWidth  = cellWidthPxDefault
	cellHeight = cellHeightPxDefault
)

// ImageRenderer holds rendering options
type ImageRenderer struct {
	width, height int
	mode          string
	invert        bool
	char          string
	rotate        int
	asciiChars    string
	dither        bool
}

// rgbTo256 converts RGB to 256-color terminal code with grayscale ramp.
// It uses pure integer arithmetic: the original float64 rounding
// (round(avg*23/255) and round(c*5/255)) is replaced by the equivalent
// round-half-up integer formula (2*x+255)/510, which avoids per-pixel float
// conversions and math.Max/math.Abs calls on the hot render path.
func rgbTo256(r, g, b uint8) int {
	ri, gi, bi := int(r), int(g), int(b)
	sum := ri + gi + bi

	// Grayscale detection: a colour is "close to grayscale" when each channel is
	// within 10 of the mean. |c - (r+g+b)/3| <= 10 is exactly equivalent to
	// |3c - (r+g+b)| <= 30, so the threshold stays exact in integer arithmetic
	// (originally computed with float64).
	maxDiff3 := 3*ri - sum
	if maxDiff3 < 0 {
		maxDiff3 = -maxDiff3
	}
	d := 3*gi - sum
	if d < 0 {
		d = -d
	}
	if d > maxDiff3 {
		maxDiff3 = d
	}
	d = 3*bi - sum
	if d < 0 {
		d = -d
	}
	if d > maxDiff3 {
		maxDiff3 = d
	}

	// if all channels are within ~10 of each other, use grayscale ramp (232-255)
	if maxDiff3 <= 30 {
		// gray = round((sum/3) * 23 / 255), computed exactly in integers so the
		// result matches the original math.Round(float64) implementation.
		gray := (46*sum + 765) / 1530
		if gray > 23 {
			gray = 23
		}
		return 232 + gray
	}

	r6 := (ri*10 + 255) / 510
	g6 := (gi*10 + 255) / 510
	b6 := (bi*10 + 255) / 510
	if r6 > 5 {
		r6 = 5
	}
	if g6 > 5 {
		g6 = 5
	}
	if b6 > 5 {
		b6 = 5
	}
	return 16 + 36*r6 + 6*g6 + b6
}

// quantize maps a color channel value to the nearest of `levels` steps
// (e.g. levels=6 spans 0..255 in 51-unit increments), used by dithering.
func quantize(val float64, levels int) float64 {
	step := 255.0 / float64(levels-1)
	return math.Round(val/step) * step
}

// applyFloydSteinberg applies Floyd-Steinberg error diffusion dithering. The
// image is quantized to 6 levels per channel so the result matches the 6x6x6
// 256-color cube used by the "256" terminal mode. Per-row error buffers
// propagate the quantization error to neighbouring pixels (right, and down-left /
// down / down-right). It is O(width*height) and allocates three float buffers.
func applyFloydSteinberg(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// quantize to 6 levels per channel (matches 256-color cube)
	const levels = 6

	// Read the source pixels directly when the concrete type is known, to avoid
	// the per-pixel interface dispatch of img.At.
	srcNRGBA, _ := img.(*image.NRGBA)
	srcRGBA, _ := img.(*image.RGBA)

	newImg := image.NewNRGBA(bounds)

	// Two flat error buffers (current row + next row), each holding R, G and B
	// segments of length w+2. Flat slices replace the previous [][]float64
	// (h separate allocations per channel) and keep the working set contiguous
	// for better cache locality.
	span := w + 2
	errCur := make([]float64, span*3)
	errNext := make([]float64, span*3)
	oR, oG, oB := 0, span, span*2

	for y := 0; y < h; y++ {
		rowBase := (bounds.Min.Y+y)*newImg.Stride + bounds.Min.X*4
		for x := 0; x < w; x++ {
			var r0, g0, b0 uint8
			if srcNRGBA != nil {
				i := (bounds.Min.Y+y)*srcNRGBA.Stride + (bounds.Min.X+x)*4
				r0, g0, b0 = srcNRGBA.Pix[i], srcNRGBA.Pix[i+1], srcNRGBA.Pix[i+2]
			} else if srcRGBA != nil {
				i := (bounds.Min.Y+y)*srcRGBA.Stride + (bounds.Min.X+x)*4
				r0, g0, b0 = srcRGBA.Pix[i], srcRGBA.Pix[i+1], srcRGBA.Pix[i+2]
			} else {
				rr, gg, bb, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
				r0, g0, b0 = uint8(rr>>8), uint8(gg>>8), uint8(bb>>8)
			}

			oldR := float64(r0) + errCur[oR+x]
			oldG := float64(g0) + errCur[oG+x]
			oldB := float64(b0) + errCur[oB+x]

			if oldR > 255 {
				oldR = 255
			} else if oldR < 0 {
				oldR = 0
			}
			if oldG > 255 {
				oldG = 255
			} else if oldG < 0 {
				oldG = 0
			}
			if oldB > 255 {
				oldB = 255
			} else if oldB < 0 {
				oldB = 0
			}

			newR := quantize(oldR, levels)
			newG := quantize(oldG, levels)
			newB := quantize(oldB, levels)

			i := rowBase + x*4
			newImg.Pix[i] = uint8(newR)
			newImg.Pix[i+1] = uint8(newG)
			newImg.Pix[i+2] = uint8(newB)
			newImg.Pix[i+3] = 255

			diffR := oldR - newR
			diffG := oldG - newG
			diffB := oldB - newB

			// right (current row)
			errCur[oR+x+1] += diffR * 7 / 16
			errCur[oG+x+1] += diffG * 7 / 16
			errCur[oB+x+1] += diffB * 7 / 16
			// down-left / down / down-right (next row)
			if x > 0 {
				errNext[oR+x-1] += diffR * 3 / 16
				errNext[oG+x-1] += diffG * 3 / 16
				errNext[oB+x-1] += diffB * 3 / 16
			}
			errNext[oR+x] += diffR * 5 / 16
			errNext[oG+x] += diffG * 5 / 16
			errNext[oB+x] += diffB * 5 / 16
			if x+1 < w {
				errNext[oR+x+1] += diffR * 1 / 16
				errNext[oG+x+1] += diffG * 1 / 16
				errNext[oB+x+1] += diffB * 1 / 16
			}
		}
		// The next row becomes the current row; the now-stale buffer is cleared.
		errCur, errNext = errNext, errCur
		for k := range errNext {
			errNext[k] = 0
		}
	}

	return newImg
}

// calculateSize computes the effective width/height from explicit flags.
//
// For tgp/sixel (isTGP=true) the values are pixel dimensions; for text modes
// (isTGP=false) width is a column count and height is a row count, where each
// character cell covers two vertical source pixels (hence the /2 factor).
// The global scale factor multiplies the resulting dimensions.
func calculateSize(img image.Image, width, height int, isTGP bool) (int, int) {
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()

	if imgW == 0 || imgH == 0 {
		return width, height
	}

	if width > 0 && height == 0 {
		if isTGP {
			height = int(math.Round(float64(imgH) * float64(width) / float64(imgW)))
		} else {
			height = int(math.Round(float64(imgH) * float64(width) / float64(imgW) / 2))
		}
	} else if height > 0 && width == 0 {
		if isTGP {
			width = int(math.Round(float64(imgW) * float64(height) / float64(imgH)))
		} else {
			width = int(math.Round(float64(imgW) * float64(height*2) / float64(imgH)))
		}
	} else if width == 0 && height == 0 {
		width = 40
		if isTGP {
			height = int(math.Round(float64(imgH) * float64(width) / float64(imgW)))
		} else {
			height = int(math.Round(float64(imgH) * float64(width) / float64(imgW) / 2))
		}
	}

	if scale != 1 {
		width = int(math.Round(float64(width) * scale))
		height = int(math.Round(float64(height) * scale))
		if width < 1 {
			width = 1
		}
		if height < 1 {
			height = 1
		}
	}

	return width, height
}

func getTerminalSize() (int, int) {
	fd := int(os.Stdout.Fd())
	if !isatty.IsTerminal(uintptr(fd)) && !isatty.IsCygwinTerminal(uintptr(fd)) {
		if tty, err := os.Open("/dev/tty"); err == nil {
			fd = int(tty.Fd())
			tty.Close()
		} else if runtime.GOOS == "windows" {
			if tty, err := os.Open("CON"); err == nil {
				fd = int(tty.Fd())
				tty.Close()
			}
		}
	}
	w, h, err := term.GetSize(fd)
	if err != nil {
		return 80, 24
	}
	return w, h
}

// isValidRotate checks if rotation value is valid (0, 90, 180, 270, 360)
func isValidRotate(degrees int) bool {
	return degrees%90 == 0 && degrees >= 0 && degrees <= 360
}

// rotateImage rotates the image by degrees
func rotateImage(img image.Image, degrees int) image.Image {
	// normalize to 0-270 range (360 == 0)
	effective := degrees % 360
	switch effective {
	case 90:
		return imaging.Rotate90(img)
	case 180:
		return imaging.Rotate180(img)
	case 270:
		return imaging.Rotate270(img)
	default:
		return img
	}
}

// sixelSupported reports whether the current terminal can display the Sixel
// graphics protocol. Terminals that do not support it silently ignore the
// escape sequence, which looks like "no output" to the user. We only return
// false when we are confident the terminal cannot render Sixel; otherwise we
// let the encoder try so that genuinely capable terminals keep working.
func sixelSupported() bool {
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "Apple_Terminal", "Terminal":
		return false
	}
	return true
}

// resolveMode maps the "auto" mode to a concrete rendering mode based on the
// current terminal's advertised capabilities. Graphics protocols (TGP/Kitty,
// then Sixel) are preferred for quality; otherwise we fall back to the best
// text mode supported by the terminal's color depth. Non-auto modes are
// returned unchanged.
func resolveMode(m string) string {
	if m != "auto" {
		return m
	}

	termProg := strings.ToLower(os.Getenv("TERM_PROGRAM"))
	term := strings.ToLower(os.Getenv("TERM"))

	// Kitty / iTerm2 / Ghostty / WezTerm support the TGP (Kitty) protocol.
	if termProg == "iterm.app" || termProg == "ghostty" || termProg == "wezterm" ||
		term == "xterm-kitty" || os.Getenv("KITTY_WINDOW_ID") != "" {
		return "tgp"
	}

	// Sixel-capable terminals (xterm, mlterm, foot, etc.) when not already
	// claimed by TGP above.
	if sixelSupported() {
		return "sixel"
	}

	// Text modes, chosen by color depth.
	if supportsTrueColor() {
		return "rgb"
	}
	if supports256Color() {
		return "256"
	}
	return "grayscale"
}

// supportsTrueColor reports whether the terminal advertises 24-bit color
// (via COLORTERM=truecolor|24bit).
func supportsTrueColor() bool {
	ct := os.Getenv("COLORTERM")
	return strings.Contains(ct, "truecolor") || strings.Contains(ct, "24bit")
}

// supports256Color reports whether the terminal advertises a 256-color palette
// (via TERM=*-256color or similar).
func supports256Color() bool {
	t := os.Getenv("TERM")
	return strings.Contains(t, "256color") || strings.Contains(t, "256")
}

// calculateTGPSize computes pixel dimensions for TGP display
func calculateTGPSize(imgW, imgH, w, h, termW, termH, statusLines int) (int, int) {
	if termW <= 0 || termW > maxWidth {
		termW = 80
	}
	if termH <= 0 || termH > maxHeight {
		termH = 24
	}

	cellW, cellH := getCellSize()
	termPixelW := termW * cellW
	termPixelH := (termH - statusLines) * cellH

	if w == 0 && h == 0 {
		w = termPixelW
		if imgW > 0 {
			h = int(math.Round(float64(imgH) * float64(w) / float64(imgW)))
		}
		if h > termPixelH {
			h = termPixelH
			if imgH > 0 {
				w = int(math.Round(float64(imgW) * float64(h) / float64(imgH)))
			}
		}
	} else if w > 0 && h == 0 {
		if imgW > 0 {
			h = int(math.Round(float64(imgH) * float64(w) / float64(imgW)))
		}
	} else if h > 0 && w == 0 {
		if imgH > 0 {
			w = int(math.Round(float64(imgW) * float64(h) / float64(imgH)))
		}
	}

	// Apply the user-supplied scale factor.
	if scale != 1 {
		w = int(math.Round(float64(w) * scale))
		h = int(math.Round(float64(h) * scale))
		if w < 1 {
			w = 1
		}
		if h < 1 {
			h = 1
		}
	}

	// Cap to a sane absolute maximum so an extreme --scale or --width cannot
	// produce a gigantic image. Unlike before, this does NOT clamp to the
	// terminal, so --fit/--scale/--width are free to enlarge the image beyond
	// the terminal (matching the text/ASCII behaviour).
	if w > maxWidth {
		w = maxWidth
	}
	if h > maxHeight {
		h = maxHeight
	}

	return w, h
}

// printTGP prints image in iTerm2/Kitty using inline image protocol
const chunkSize = 4096

// prepareImage applies rotation, inversion and resizing, returning the image
// ready for protocol-specific encoding. When width or height is <= 0 the image
// is only rotated/inverted (no resize).
func prepareImage(img image.Image, width, height, rotate int, invert bool) image.Image {
	img = rotateImage(img, rotate)
	if invert {
		img = imaging.Invert(img)
	}
	if width > 0 && height > 0 {
		resized := imaging.Resize(img, width, height, imaging.Lanczos)
		if resized == nil {
			log.Fatalf("Failed to resize image: result is nil")
		}
		return resized
	}
	return img
}

func printTGP(img image.Image, width, height int, rotate int, invert bool) {
	term := os.Getenv("TERM_PROGRAM")

	resized := prepareImage(img, width, height, rotate, invert)

	// Encode the resized image into PNG format
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, resized); err != nil {
		log.Fatalf("Failed to encode image: %v", err)
	}

	// Convert PNG bytes to base64 for terminal protocols
	data := base64.StdEncoding.EncodeToString(buf.Bytes())

	switch term {
	case "iTerm.app":
		fmt.Printf("\033]1337;File=name=inline.png;width=auto;height=auto;inline=1:%s\a\n", data)
	case "Ghostty", "WezTerm":
		printTGPKitty(data)
	default:
		printTGPKitty(data)
	}
}

func printTGPKitty(data string) {
	for i := 0; i < len(data); i += chunkSize {
		end := i + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[i:end]

		more := 0
		if end < len(data) {
			more = 1
		}

		if i == 0 {
			fmt.Printf("\033_Ga=T,f=100,t=d,m=%d;%s\033\\", more, chunk)
		} else {
			fmt.Printf("\033_Gm=%d;%s\033\\", more, chunk)
		}
	}
	fmt.Print("\n")
}

// printSixel renders the image through the Sixel graphics protocol, which is
// supported by xterm, mlterm, WezTerm and Ghostty. It requires a Sixel-capable
// terminal; see sixelSupported for the guard used by callers.
func printSixel(img image.Image, width, height int, rotate int, invert bool) {
	resized := prepareImage(img, width, height, rotate, invert)

	enc := sixel.NewEncoder(os.Stdout)
	enc.Width = width
	enc.Height = height
	enc.Dither = dither
	enc.Colors = 256

	if err := enc.Encode(resized); err != nil {
		log.Fatalf("Failed to encode sixel: %v", err)
	}
}

// renderTerminal converts the image into a slice of ANSI escape-coded strings,
// one per terminal row. It uses the half-block character (▀) so that each cell
// shows two vertical source pixels (top in the foreground color, bottom in the
// background color). The image is therefore resized to width x (height*2) so
// that one character row maps to two pixel rows. Dithering, when enabled, is
// applied before resizing.
func (r *ImageRenderer) renderTerminal(img image.Image) []string {
	img = rotateImage(img, r.rotate)

	if r.dither {
		img = applyFloydSteinberg(img)
	}

	// imaging.Resize always returns *image.NRGBA, so we can read its pixels
	// directly without the per-pixel interface dispatch of resized.At.
	resized := imaging.Resize(img, r.width, r.height*2, imaging.Lanczos)
	rgba := resized
	bounds := resized.Bounds()
	minX, minY := bounds.Min.X, bounds.Min.Y
	dx, dy := bounds.Dx(), bounds.Dy()
	lines := make([]string, 0, dy/2)

	// Resolve the draw character and the ascii ramp once, outside the pixel loop.
	charToUse := r.char
	if charToUse == "" {
		charToUse = "▀"
	}

	var asciiRunes []rune
	if r.mode == "ascii" {
		chars := r.asciiChars
		if r.char != "" && r.char != "▀" {
			chars = r.char
		}
		if len(chars) == 0 {
			chars = "@"
		}
		asciiRunes = []rune(chars)
	}

	// buf is reused for integer-to-ASCII conversion of escape-sequence numbers;
	// strings.Builder copies the bytes on Write, so the buffer is safe to reuse.
	var buf [16]byte
	for y := minY; y < minY+dy; y += 2 {
		var sb strings.Builder
		sb.Grow(dx * 30)
		rowBelow := y + 1
		for x := minX; x < minX+dx; x++ {
			var tr8, tg8, tb8, br8, bg8, bb8 uint8
			// Direct pixel access avoids the per-pixel interface dispatch
			// and color.Color boxing that resized.At(x, y) would incur.
			i := y*rgba.Stride + x*4
			tr8, tg8, tb8 = rgba.Pix[i], rgba.Pix[i+1], rgba.Pix[i+2]
			if rowBelow < minY+dy {
				j := rowBelow*rgba.Stride + x*4
				br8, bg8, bb8 = rgba.Pix[j], rgba.Pix[j+1], rgba.Pix[j+2]
			} else {
				br8, bg8, bb8 = tr8, tg8, tb8
			}

			if r.invert {
				tr8, tg8, tb8 = 255-tr8, 255-tg8, 255-tb8
				br8, bg8, bb8 = 255-br8, 255-bg8, 255-bb8
			}

			switch r.mode {
			case "ascii":
				grayTop := 0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8)
				grayBottom := 0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8)
				avgGray := (grayTop + grayBottom) / 2
				index := int(avgGray * float64(len(asciiRunes)) / 255)
				if index >= len(asciiRunes) {
					index = len(asciiRunes) - 1
				}
				sb.WriteRune(asciiRunes[index])
			case "256":
				sb.WriteString("\033[38;5;")
				sb.Write(strconv.AppendInt(buf[:0], int64(rgbTo256(tr8, tg8, tb8)), 10))
				sb.WriteString(";48;5;")
				sb.Write(strconv.AppendInt(buf[:0], int64(rgbTo256(br8, bg8, bb8)), 10))
				sb.WriteByte('m')
				sb.WriteString(charToUse)
			case "grayscale":
				grayTop := uint8(0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8))
				grayBottom := uint8(0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8))
				sb.WriteString("\033[38;2;")
				sb.Write(strconv.AppendInt(buf[:0], int64(grayTop), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(grayTop), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(grayTop), 10))
				sb.WriteString(";48;2;")
				sb.Write(strconv.AppendInt(buf[:0], int64(grayBottom), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(grayBottom), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(grayBottom), 10))
				sb.WriteByte('m')
				sb.WriteString(charToUse)
			default: // rgb
				sb.WriteString("\033[38;2;")
				sb.Write(strconv.AppendInt(buf[:0], int64(tr8), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(tg8), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(tb8), 10))
				sb.WriteString(";48;2;")
				sb.Write(strconv.AppendInt(buf[:0], int64(br8), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(bg8), 10))
				sb.WriteByte(';')
				sb.Write(strconv.AppendInt(buf[:0], int64(bb8), 10))
				sb.WriteByte('m')
				sb.WriteString(charToUse)
			}
		}

		if r.mode != "ascii" {
			sb.WriteString("\033[0m")
		}
		lines = append(lines, sb.String())
	}

	return lines
}

func createLink(url, text string) string {
	if !isatty.IsTerminal(uintptr(os.Stdout.Fd())) && !isatty.IsCygwinTerminal(uintptr(os.Stdout.Fd())) {
		return url
	}
	return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
}

// printCopyleftBody prints the project links (homepage, youtube, donation).
func printCopyleftBody() {
	fmt.Println("Homepage: ", createLink("https://dotoca.net/pixu", "https://dotoca.net/pixu"))
	fmt.Println("Youtube:  ", createLink("https://youtube.com/@xvoland", "https://youtube.com/@xvoland"))
	fmt.Println("Donation: ", createLink("https://paypal.me/xvoland", "https://paypal.me/xvoland"))
}

// printCopyleft prints the full copyleft block (used by the QR display).
func printCopyleft() {
	fmt.Println("")
	printCopyleftBody()
	year := time.Now().Year()
	fmt.Printf("Copyright © %d, Vitalii Tereshchuk / xVoLAnD | URL: %s\n",
		year, createLink("https://dotoca.net", "DOTOCA.NET"))
}

// applyEnvDefaults sets flag values from environment variables,
// but only for flags not explicitly set by the user on the command line.
// Priority: CLI flag > env variable > default
func applyEnvDefaults(cmd *cobra.Command) {
	type envMapping struct {
		env string
		set func(string)
	}

	mappings := []envMapping{
		{"PIXU_WIDTH", func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				width = n
			}
		}},
		{"PIXU_HEIGHT", func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				height = n
			}
		}},
		{"PIXU_MODE", func(v string) { mode = v }},
		{"PIXU_INVERT", func(v string) {
			if v == "1" || v == "true" || v == "TRUE" {
				invert = true
			} else if v == "0" || v == "false" || v == "FALSE" {
				invert = false
			}
		}},
		{"PIXU_CHAR", func(v string) { char = v }},
		{"PIXU_ROTATE", func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				rotate = n
			}
		}},
		{"PIXU_ASCII_CHARS", func(v string) {
			if v != "" {
				asciiChars = v
			}
		}},
		{"PIXU_CELL_WIDTH", func(v string) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cellWidth = n
			}
		}},
		{"PIXU_CELL_HEIGHT", func(v string) {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				cellHeight = n
			}
		}},
		{"PIXU_SCALE", func(v string) {
			if f, err := strconv.ParseFloat(v, 64); err == nil {
				scale = f
			}
		}},
		{"PIXU_DITHER", func(v string) {
			if v == "1" || v == "true" || v == "TRUE" {
				dither = true
			} else if v == "0" || v == "false" || v == "FALSE" {
				dither = false
			}
		}},
	}

	flagToEnv := map[string]string{
		"width":  "PIXU_WIDTH",
		"height": "PIXU_HEIGHT",
		"mode":   "PIXU_MODE",
		"invert": "PIXU_INVERT",
		"char":   "PIXU_CHAR",
		"rotate": "PIXU_ROTATE",
		"scale":  "PIXU_SCALE",
		"dither": "PIXU_DITHER",
	}

	for _, m := range mappings {
		val := os.Getenv(m.env)
		if val == "" {
			continue
		}

		// skip if the corresponding CLI flag was explicitly set by the user
		var flagName string
		for fn, ev := range flagToEnv {
			if ev == m.env {
				flagName = fn
				break
			}
		}
		if flagName != "" && cmd.Flags().Changed(flagName) {
			continue
		}

		m.set(val)
	}
}

func runInteractiveMode(args []string, mode string, invert bool, rotate int, char string, width int, height int, dither bool, asciiChars string) {
	imageExtensions := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true,
	}

	files := args
	if len(files) == 1 {
		if isDir, _ := pathExists(files[0]); isDir {
			dirPath := files[0]
			entries, err := os.ReadDir(dirPath)
			if err != nil {
				fmt.Printf("Error reading directory: %v\n", err)
				return
			}
			files = nil
			for _, e := range entries {
				if !e.IsDir() {
					ext := strings.ToLower(filepath.Ext(e.Name()))
					if imageExtensions[ext] {
						files = append(files, filepath.Join(dirPath, e.Name()))
					}
				}
			}
		}
	}

	if len(files) == 0 {
		fmt.Println("No images found")
		return
	}

	currentIndex := 0

	showImage := func() {
		termW, termH := getTerminalSize()

		fmt.Print("\033[1;0H\033[J")

		img, err := imaging.Open(files[currentIndex], imaging.AutoOrientation(true))
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		img = rotateImage(img, rotate)
		if invert {
			img = imaging.Invert(img)
		}

		bounds := img.Bounds()
		imgW := bounds.Dx()
		imgH := bounds.Dy()

		if mode == "tgp" {
			displayW, displayH := calculateTGPSize(imgW, imgH, width, height, termW, termH, statusLinesInteractive)

			resized := imaging.Resize(img, displayW, displayH, imaging.Lanczos)
			if resized == nil {
				fmt.Printf("Error: failed to resize image\n")
				return
			}
			buf := new(bytes.Buffer)
			if err := png.Encode(buf, resized); err != nil {
				fmt.Printf("Error encoding image: %v\n", err)
				return
			}
			printTGPKitty(base64.StdEncoding.EncodeToString(buf.Bytes()))

			_, cellH := getCellSize()
			termRowH := displayH/cellH + 3
			fmt.Printf("\r\033[%dH\033[7m%s | %dx%d | TGP | %d/%d\033[0m\n",
				termRowH, filepath.Base(files[currentIndex]), imgW, imgH, currentIndex+1, len(files))
			fmt.Printf("\033[%dH\033[33m←/→: prev/next | ESC/Ctrl+C: quit\033[0m\n", termRowH+1)
			return
		}

		if mode == "sixel" {
			if !sixelSupported() {
				fmt.Printf("\033[7mSixel not supported by this terminal (%s)\033[0m\n", os.Getenv("TERM_PROGRAM"))
				return
			}
			displayW, displayH := calculateTGPSize(imgW, imgH, width, height, termW, termH, statusLinesInteractive)
			resized := imaging.Resize(img, displayW, displayH, imaging.Lanczos)
			if resized == nil {
				fmt.Printf("Error: failed to resize image\n")
				return
			}
			enc := sixel.NewEncoder(os.Stdout)
			enc.Width = displayW
			enc.Height = displayH
			enc.Dither = dither
			enc.Colors = 256
			if err := enc.Encode(resized); err != nil {
				fmt.Printf("Error encoding sixel: %v\n", err)
				return
			}
			_, cellH := getCellSize()
			termRowH := displayH/cellH + 3
			fmt.Printf("\r\033[%dH\033[7m%s | %dx%d | SIXEL | %d/%d\033[0m\n",
				termRowH, filepath.Base(files[currentIndex]), imgW, imgH, currentIndex+1, len(files))
			fmt.Printf("\033[%dH\033[33m←/→: prev/next | ESC/Ctrl+C: quit\033[0m\n", termRowH+1)
			return
		}

		displayW := termW
		var displayH int
		if width > 0 && height == 0 {
			displayW = width
			if imgW > 0 {
				displayH = width * imgH / imgW / 2
			}
		} else if height > 0 && width == 0 {
			displayH = height
			if imgH > 0 {
				displayW = height * 2 * imgW / imgH
			}
		} else if width > 0 && height > 0 {
			displayW = width
			displayH = height
		} else {
			if imgW > 0 {
				displayH = termW * imgH / imgW / 2
			} else {
				displayH = termH - statusLinesTerminal
			}
		}

		if imgW > 0 && displayH > termH-statusLinesTerminal {
			displayH = termH - statusLinesTerminal
			displayW = displayH * 2 * imgW / imgH
		}
		if displayW > termW {
			displayW = termW
			if imgW > 0 {
				displayH = displayW * imgH / imgW / 2
			}
		}

		renderer := &ImageRenderer{
			width:      displayW,
			height:     displayH,
			mode:       mode,
			invert:     invert,
			char:       char,
			rotate:     rotate,
			asciiChars: asciiChars,
			dither:     dither,
		}
		if mode == "ascii" && char != "" && char != "▀" {
			renderer.asciiChars = char
		}
		lines := renderer.renderTerminal(img)
		dispH := len(lines)

		// Assemble the whole frame (image rows + status lines) into one buffer and
		// write it to the terminal in a single call instead of many fmt.Print
		// syscalls per navigation redraw.
		var buf bytes.Buffer
		buf.WriteString("\033[1;0H")
		for _, line := range lines {
			buf.WriteByte('\r')
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
		fmt.Fprintf(&buf, "\r\033[%dH\033[7m%s | %dx%d | %s | %d/%d\033[0m\n",
			dispH+1, filepath.Base(files[currentIndex]), imgW, imgH, strings.ToUpper(mode), currentIndex+1, len(files))
		fmt.Fprintf(&buf, "\r\033[%dH\033[33m←/→: prev/next | ESC/Ctrl+C: quit\033[0m\n", dispH+2)
		os.Stdout.Write(buf.Bytes())
	}

	showImage()

	ttyPath := "/dev/tty"
	if runtime.GOOS == "windows" {
		ttyPath = "CON"
	}
	tty, err := os.Open(ttyPath)
	if err != nil {
		fmt.Println("Error: cannot open terminal for input")
		return
	}
	defer tty.Close()

	oldState, err := term.MakeRaw(int(tty.Fd()))
	if err != nil {
		fmt.Println("Error: cannot set raw mode")
		return
	}
	defer term.Restore(int(tty.Fd()), oldState)

	for {
		buf := make([]byte, 10)
		n, err := tty.Read(buf)
		if err != nil || n == 0 {
			continue
		}

		if buf[0] == 27 {
			if n == 1 {
				fmt.Print("\033[2J\033[H")
				return
			}
			if n >= 3 && buf[1] == 91 {
				switch buf[2] {
				case 67:
					if currentIndex < len(files)-1 {
						currentIndex++
						showImage()
					}
				case 68:
					if currentIndex > 0 {
						currentIndex--
						showImage()
					}
				}
			}
			continue
		}

		if buf[0] == 3 {
			fmt.Print("\033[2J\033[H")
			return
		}

		switch buf[0] {
		case 'q', 'Q':
			fmt.Print("\033[2J\033[H")
			return
		case 'n', 'N', 14:
			if currentIndex < len(files)-1 {
				currentIndex++
				showImage()
			}
		case 'p', 'P', 16:
			if currentIndex > 0 {
				currentIndex--
				showImage()
			}
		}
	}
}

func showQRCode() {
	if len(qrCodeData) == 0 {
		fmt.Println("QR code is not available in this build.")
		fmt.Println("To support PIXU, visit: https://paypal.me/xvoland")
		printCopyleft()
		return
	}

	img, err := imaging.Decode(bytes.NewReader(qrCodeData), imaging.AutoOrientation(true))
	if err != nil {
		fmt.Println("Error decoding QR:", err)
		return
	}

	bounds := img.Bounds()
	displayW := bounds.Dx()
	displayH := bounds.Dy()

	resized := imaging.Resize(img, displayW, displayH, imaging.Lanczos)

	term := os.Getenv("TERM_PROGRAM")

	switch term {
	case "iTerm.app":
		buf := new(bytes.Buffer)
		if err := png.Encode(buf, resized); err != nil {
			log.Fatalf("Failed to encode QR image: %v", err)
		}
		encodedData := base64.StdEncoding.EncodeToString(buf.Bytes())
		fmt.Printf("\033]1337;File=name=qr.png;width=auto;height=auto;inline=1:%s\a\n", encodedData)
	default:
		buf := new(bytes.Buffer)
		if err := png.Encode(buf, resized); err != nil {
			log.Fatalf("Failed to encode QR image: %v", err)
		}
		printTGPKitty(base64.StdEncoding.EncodeToString(buf.Bytes()))
	}

	if qrCodeText != "" {
		fmt.Println()
		fmt.Println(qrCodeText)
	}
}

func pathExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		return info.IsDir(), nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// validateFlags checks all flag values are valid
func validateFlags() {
	validModes := map[string]bool{"rgb": true, "grayscale": true, "256": true, "ascii": true, "tgp": true, "sixel": true, "auto": true}
	if !validModes[mode] {
		log.Fatalf("Invalid mode: %q (must be rgb, grayscale, 256, ascii, tgp, sixel, or auto)", mode)
	}
	mode = resolveMode(mode)

	if !isValidRotate(rotate) {
		log.Fatalf("Invalid rotate value: %d (must be 0, 90, 180, 270, or 360)", rotate)
	}

	if width < 0 || height < 0 {
		log.Fatalf("Invalid width or height: width=%d height=%d (must be >= 0)", width, height)
	}
	if width > maxWidth || height > maxHeight {
		log.Fatalf("Width or height too large: width=%d height=%d (max %d)", width, height, maxWidth)
	}

	if scale <= 0 {
		log.Fatalf("Invalid scale: %v (must be > 0)", scale)
	}
}

// loadImage loads image from clipboard, input, stdin, or file argument
func loadImage(args []string) image.Image {
	var img image.Image
	var err error

	if paste {
		// 1. Try binary image data (screenshot, Photoshop, etc.)
		clipData := clipboardReadImage()
		if len(clipData) > 0 {
			img, err = imaging.Decode(bytes.NewReader(clipData), imaging.AutoOrientation(true))
			if err == nil {
				return img
			}
		}

		// 2. Try text content (file path, URL, or base64)
		clipText := strings.TrimSpace(clipboardReadText())
		if clipText == "" {
			log.Fatalf("Clipboard is empty")
		}

		// 2a. File path
		if isDir, _ := pathExists(clipText); !isDir {
			if _, err := os.Stat(clipText); err == nil {
				ext := strings.ToLower(filepath.Ext(clipText))
				imageExts := map[string]bool{".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true}
				if imageExts[ext] {
					img, err = imaging.Open(clipText, imaging.AutoOrientation(true))
					if err == nil {
						return img
					}
				}
			}
		}

		// 2b. URL (http/https)
		if u, err := url.Parse(clipText); err == nil && (u.Scheme == "http" || u.Scheme == "https") {
			resp, err := httpClient.Get(clipText)
			if err == nil {
				defer resp.Body.Close()
				if data, err := io.ReadAll(resp.Body); err == nil && len(data) > 0 {
					img, err = imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
					if err == nil {
						return img
					}
				}
			}
		}

		// 2c. Base64 (with or without data:image/...;base64, prefix)
		b64Data := clipText
		if idx := strings.Index(clipText, ";base64,"); idx != -1 {
			b64Data = clipText[idx+8:]
		}
		if decoded, err := base64.StdEncoding.DecodeString(b64Data); err == nil && len(decoded) > 0 {
			img, err = imaging.Decode(bytes.NewReader(decoded), imaging.AutoOrientation(true))
			if err == nil {
				return img
			}
		}
		if decoded, err := base64.URLEncoding.DecodeString(b64Data); err == nil && len(decoded) > 0 {
			img, err = imaging.Decode(bytes.NewReader(decoded), imaging.AutoOrientation(true))
			if err == nil {
				return img
			}
		}

		// 2d. Try decoding raw text bytes as image
		img, err = imaging.Decode(bytes.NewReader([]byte(clipText)), imaging.AutoOrientation(true))
		if err == nil {
			return img
		}

		log.Fatalf("Cannot decode image from clipboard content")
	}

	if input != "" {
		var data []byte
		if input == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(input)
		}
		if err != nil {
			log.Fatalf("Error reading input: %v", err)
		}
		img, err = imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
		if err != nil {
			log.Fatalf("Error decoding image: %v", err)
		}
		return img
	}

	if len(args) > 0 && args[0] == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			log.Fatalf("Error reading stdin: %v", err)
		}
		img, err = imaging.Decode(bytes.NewReader(data), imaging.AutoOrientation(true))
		if err != nil {
			log.Fatalf("Error decoding image from stdin: %v", err)
		}
		return img
	}

	if len(args) == 0 {
		return nil
	}

	img, err = imaging.Open(args[0], imaging.AutoOrientation(true))
	if err != nil {
		log.Fatalf("Error loading image: %v", err)
	}
	return img
}

// fitToTerminal computes the render width/height for --fit while preserving the
// image aspect ratio. axis "W" fits by terminal width, "H" (or empty) fits by
// terminal height. scale is a multiplier applied to the fitted size (1 = full
// terminal fit, 0.5 = half). Text modes render each character row as two
// vertical pixels (half-block), so the effective pixel height is 2x the row count.
func fitToTerminal(imgW, imgH, termW, termH int, mode, axis string, scale float64) (width, height int) {
	if imgW <= 0 || imgH <= 0 {
		// Unknown aspect: just fill the available terminal area.
		if mode == "tgp" || mode == "sixel" {
			cellW, cellH := getCellSize()
			width = termW * cellW
			height = (termH - statusLinesTGP) * cellH
		} else {
			width = termW
			height = termH - statusLinesTerminal
		}
		return applyFitScale(width, height, scale)
	}

	if mode == "tgp" || mode == "sixel" {
		cellW, cellH := getCellSize()
		termPixelW := termW * cellW
		termPixelH := (termH - statusLinesTGP) * cellH
		// Lock the chosen dimension to the terminal; the other follows by aspect
		// ratio and may overflow (the user explicitly prioritized this axis).
		if axis == "W" {
			width = termPixelW
			height = int(math.Round(float64(width) * float64(imgH) / float64(imgW)))
		} else { // "H" or default: fill the height
			height = termPixelH
			width = int(math.Round(float64(height) * float64(imgW) / float64(imgH)))
		}
		return applyFitScale(width, height, scale)
	}

	// Text modes (half-block): one row covers two vertical pixels.
	availH := termH - statusLinesTerminal
	// Lock the chosen dimension to the terminal; the other follows by aspect
	// ratio and may overflow (the user explicitly prioritized this axis).
	if axis == "W" {
		width = termW
		height = int(math.Round(float64(width) * float64(imgH) / (2 * float64(imgW))))
	} else { // "H" or default: fill the height
		height = availH
		width = int(math.Round(2 * float64(height) * float64(imgW) / float64(imgH)))
	}
	return applyFitScale(width, height, scale)
}

// applyFitScale multiplies the fitted width/height by scale (a no-op when scale==1).
func applyFitScale(width, height int, scale float64) (int, int) {
	if scale == 1 {
		return width, height
	}
	return int(math.Round(float64(width) * scale)), int(math.Round(float64(height) * scale))
}

// parseFit interprets the --fit flag value: empty/"H" => fit by height with
// scale 1, "W" => fit by width with scale 1, and a positive number is a scale
// factor applied to the fitted size (1 = full terminal fit, 0.5 = half).
func parseFit(fit string) (axis string, scale float64) {
	switch strings.TrimSpace(fit) {
	case "", "H", "h":
		return "H", 1.0
	case "W", "w":
		return "W", 1.0
	}
	if s, err := strconv.ParseFloat(strings.TrimSpace(fit), 64); err == nil && s > 0 {
		return "H", s
	}
	return "H", 1.0
}

// renderAndOutput renders the image and writes to output
func renderAndOutput(img image.Image) {
	// Determine the terminal size once and reuse it across the fit logic and the
	// protocol-specific size calculations instead of querying it repeatedly.
	termW, termH := getTerminalSize()

	if fit != "" {
		axis, scale := parseFit(fit)
		imgW := img.Bounds().Dx()
		imgH := img.Bounds().Dy()
		width, height = fitToTerminal(imgW, imgH, termW, termH, mode, axis, scale)
	}

	if mode == "tgp" || mode == "sixel" {
		imgW := img.Bounds().Dx()
		imgH := img.Bounds().Dy()
		cellW, cellH := getCellSize()
		var pxW, pxH int
		if fit != "" {
			axis, sc := parseFit(fit)
			pxW, pxH = fitToTerminal(imgW, imgH, termW, termH, mode, axis, sc)
		} else {
			// --width/--height are cell counts; convert to pixels so their visual
			// size matches the text/ASCII modes (where one column == one cell).
			pxW = width * cellW
			pxH = height * cellH
		}
		outW, outH := calculateTGPSize(imgW, imgH, pxW, pxH, termW, termH, statusLinesTGP)
		if mode == "tgp" {
			printTGP(img, outW, outH, rotate, invert)
			return
		}
		if !sixelSupported() {
			log.Fatalf("Sixel is not supported by this terminal (%s). "+
				"Use a Sixel-capable terminal (xterm, mlterm, WezTerm, Ghostty) "+
				"or switch to --mode tgp for iTerm2/Kitty support.", os.Getenv("TERM_PROGRAM"))
		}
		printSixel(img, outW, outH, rotate, invert)
		return
	}

	scaleWidth, scaleHeight := calculateSize(img, width, height, false)

	renderer := &ImageRenderer{
		width:      scaleWidth,
		height:     scaleHeight,
		mode:       mode,
		invert:     invert,
		char:       char,
		rotate:     rotate,
		asciiChars: asciiChars,
		dither:     dither,
	}

	var outputWriter io.Writer = os.Stdout
	if output != "" {
		f, err := os.Create(output)
		if err != nil {
			log.Fatalf("Error creating output file: %v", err)
		}
		defer f.Close()
		outputWriter = f
	}

	// Buffer every rendered line and issue a single write instead of one
	// fmt.Fprintln per line, cutting the number of syscalls on the output path.
	var buf bytes.Buffer
	for _, line := range renderer.renderTerminal(img) {
		buf.WriteString(line)
		buf.WriteByte('\n')
	}
	if _, err := outputWriter.Write(buf.Bytes()); err != nil {
		log.Fatalf("Error writing output: %v", err)
	}
}

func main() {

	rootCmd := &cobra.Command{
		Use:  "pixu",
		Args: cobra.ArbitraryArgs,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			applyEnvDefaults(cmd)
		},
		Run: func(cmd *cobra.Command, args []string) {
			if showVersion {
				printInfo()
				return
			}

			validateFlags()

			if qr {
				showQRCode()
				return
			}

			if interactive {
				runInteractiveMode(args, mode, invert, rotate, char, width, height, dither, asciiChars)
				return
			}

			img := loadImage(args)
			if img == nil {
				printInfo()
				cmd.Help()
				return
			}

			renderAndOutput(img)
		},
	}

	// Disable built-in help command
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.PersistentFlags().Bool("help", false, "")

	// Flags
	rootCmd.Flags().IntVarP(&height, "height", "h", 0, "Height in characters")
	rootCmd.Flags().IntVarP(&width, "width", "w", 0, "Width in characters")
	rootCmd.Flags().StringVarP(&mode, "mode", "m", "rgb", "Mode: rgb/grayscale/ascii/tgp/sixel/auto")
	rootCmd.Flags().BoolVarP(&invert, "invert", "i", false, "Invert colors")
	rootCmd.Flags().StringVarP(&char, "char", "c", "▀", "Block character to use")
	rootCmd.Flags().IntVarP(&rotate, "rotate", "r", 0, "Rotate: 0,90,180,270,360")
	rootCmd.Flags().StringVarP(&fit, "fit", "f", "", "Fit to terminal size: H=by height, W=by width, or a number as a scale factor (e.g. --fit 0.5, --fit 3). Bare --fit is not allowed; use --fit H for by-height")
	rootCmd.Flags().BoolVarP(&dither, "dither", "d", false, "Apply Floyd-Steinberg dithering")
	rootCmd.Flags().Float64VarP(&scale, "scale", "S", 1.0, "Scale factor (e.g. 0.5, 2); multiplies the computed size")
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "I", false, "Interactive mode with navigation and zoom")
	rootCmd.Flags().BoolVarP(&qr, "qr", "", false, "Show QR code for donation")
	rootCmd.Flags().StringVar(&input, "input", "", "Input file (use - for stdin)")
	rootCmd.Flags().BoolVarP(&paste, "paste", "p", false, "Read image from clipboard")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file (save to file instead of stdout)")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "Show version and exit")

	// version command only (like git)
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			printInfo()
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
