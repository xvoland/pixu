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
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	_ "golang.org/x/image/webp"
)

const defaultVersion = "x.x.x"

const (
	cellWidthPxDefault  = 10 // approximate terminal cell width in pixels
	cellHeightPxDefault = 20 // approximate terminal cell height in pixels

	statusLinesTGP        = 4 // status lines for TGP mode
	statusLinesTerminal   = 4 // status lines for terminal rendering (fit mode)
	statusLinesInteractive = 3 // status lines for interactive mode (excluding header)

	maxWidth  = 10000 // maximum allowed width in characters
	maxHeight = 10000 // maximum allowed height in characters
)

// getCellSize returns terminal cell size in pixels, with env override
func getCellSize() (int, int) {
	w, h := cellWidthPxDefault, cellHeightPxDefault
	if v := os.Getenv("PIXU_CELL_WIDTH"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			w = n
		}
	}
	if v := os.Getenv("PIXU_CELL_HEIGHT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			h = n
		}
	}
	return w, h
}

var qrCodeText = "Scan to support PIXU!\nhttps://paypal.me/xvoland\n"

var buildSource = "local"

//go:embed qr-code.jpg
var qrCodeData []byte

var (
	width, height, rotate int
	mode string
	invert bool
	char string
	version = defaultVersion
	showVersion bool
	fit bool
	dither bool
	interactive bool
	qr bool
	input string
	paste bool
	output string
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

// rgbTo256 converts RGB to 256-color terminal code with grayscale ramp
func rgbTo256(r, g, b uint8) int {
	// check if the color is close to grayscale
	rF, gF, bF := float64(r), float64(g), float64(b)
	avg := (rF + gF + bF) / 3
	maxDiff := math.Max(math.Abs(rF-avg), math.Max(math.Abs(gF-avg), math.Abs(bF-avg)))

	// if all channels are within ~10 of each other, use grayscale ramp (232-255)
	if maxDiff <= 10 {
		gray := int(math.Round(avg / 255 * 23))
		if gray < 0 {
			gray = 0
		} else if gray > 23 {
			gray = 23
		}
		return 232 + gray
	}

	r6 := int(math.Round(float64(r) * 5 / 255))
	g6 := int(math.Round(float64(g) * 5 / 255))
	b6 := int(math.Round(float64(b) * 5 / 255))
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

// quantize maps a color channel value to the nearest level (0..levels-1)
func quantize(val float64, levels int) float64 {
	step := 255.0 / float64(levels-1)
	return math.Round(val/step) * step
}

func applyFloydSteinberg(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	// quantize to 6 levels per channel (matches 256-color cube)
	const levels = 6

	errR := make([][]float64, h)
	errG := make([][]float64, h)
	errB := make([][]float64, h)
	for i := 0; i < h; i++ {
		errR[i] = make([]float64, w+2)
		errG[i] = make([]float64, w+2)
		errB[i] = make([]float64, w+2)
	}

	newImg := image.NewRGBA(bounds)

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, b, a := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()

			oldR := float64(r>>8) + errR[y][x]
			oldG := float64(g>>8) + errG[y][x]
			oldB := float64(b>>8) + errB[y][x]

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

			newImg.Set(x+bounds.Min.X, y+bounds.Min.Y, color.RGBA{
				R: uint8(newR), G: uint8(newG), B: uint8(newB), A: uint8(a >> 8),
			})

			diffR := oldR - newR
			diffG := oldG - newG
			diffB := oldB - newB

			if x+1 < w {
				errR[y][x+1] += diffR * 7 / 16
				errG[y][x+1] += diffG * 7 / 16
				errB[y][x+1] += diffB * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					errR[y+1][x-1] += diffR * 3 / 16
					errG[y+1][x-1] += diffG * 3 / 16
					errB[y+1][x-1] += diffB * 3 / 16
				}
				errR[y+1][x] += diffR * 5 / 16
				errG[y+1][x] += diffG * 5 / 16
				errB[y+1][x] += diffB * 5 / 16
				if x+1 < w {
					errR[y+1][x+1] += diffR * 1 / 16
					errG[y+1][x+1] += diffG * 1 / 16
					errB[y+1][x+1] += diffB * 1 / 16
				}
			}
		}
	}

	return newImg
}

// calculateSize computes width/height automatically
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

// calculateTGPSize computes pixel dimensions for TGP display
func calculateTGPSize(imgW, imgH, w, h, termW, termH, statusLines int) (int, int) {
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
		w = w * cellW
		if imgW > 0 {
			h = int(math.Round(float64(imgH) * float64(w) / float64(imgW)))
		}
	} else if h > 0 && w == 0 {
		h = h * cellH
		if imgH > 0 {
			w = int(math.Round(float64(imgW) * float64(h) / float64(imgH)))
		}
	}
	return w, h
}

// printTGP prints image in iTerm2/Kitty using inline image protocol
const chunkSize = 4096

func printTGP(img image.Image, width, height int, rotate int, invert bool) {
	term := os.Getenv("TERM_PROGRAM")

	img = rotateImage(img, rotate)

	if invert {
		img = imaging.Invert(img)
	}

	// Resize the image to the specified width and height
	// Note: Kitty/Ghostty terminal graphics can take dimensions from PNG metadata,
	// but resizing is useful to control display size manually
	resized := imaging.Resize(img, width, height, imaging.Lanczos)

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

// renderTerminal returns terminal representation lines
func (r *ImageRenderer) renderTerminal(img image.Image) []string {
	img = rotateImage(img, r.rotate)

	if r.dither {
		img = applyFloydSteinberg(img)
	}

	resized := imaging.Resize(img, r.width, r.height*2, imaging.Lanczos)
	bounds := resized.Bounds()
	lines := make([]string, 0, bounds.Dy()/2)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		var sb strings.Builder
		sb.Grow(bounds.Dx() * 30)
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			topColor := resized.At(x, y)
			bottomColor := topColor
			if y+1 < bounds.Max.Y {
				bottomColor = resized.At(x, y+1)
			}

			tr, tg, tb, _ := topColor.RGBA()
			br, bg, bb, _ := bottomColor.RGBA()
			tr8, tg8, tb8 := uint8(tr>>8), uint8(tg>>8), uint8(tb>>8)
			br8, bg8, bb8 := uint8(br>>8), uint8(bg>>8), uint8(bb>>8)

			if r.invert {
				tr8, tg8, tb8 = 255-tr8, 255-tg8, 255-tb8
				br8, bg8, bb8 = 255-br8, 255-bg8, 255-bb8
			}

			charToUse := r.char
			if charToUse == "" {
				charToUse = "▀"
			}

			switch r.mode {
			case "ascii":
				grayTop := 0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8)
				grayBottom := 0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8)
				avgGray := (grayTop + grayBottom) / 2

				chars := r.asciiChars
				if r.char != "" && r.char != "▀" {
					chars = r.char
				}

				if len(chars) == 0 {
					chars = "@"
				}

				runes := []rune(chars)
				index := int(avgGray / 255 * float64(len(runes)))
				if index >= len(runes) {
					index = len(runes) - 1
				}
				sb.WriteString(string(runes[index]))
			case "256":
				fmt.Fprintf(&sb, "\033[38;5;%d;48;5;%dm%s", rgbTo256(tr8, tg8, tb8), rgbTo256(br8, bg8, bb8), charToUse)
			case "grayscale":
				grayTop := uint8(0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8))
				grayBottom := uint8(0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8))
				fmt.Fprintf(&sb, "\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s", grayTop, grayTop, grayTop, grayBottom, grayBottom, grayBottom, charToUse)
			default: // rgb
				fmt.Fprintf(&sb, "\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s", tr8, tg8, tb8, br8, bg8, bb8, charToUse)
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

func printCopyleft() {
	year := time.Now().Year()
	fmt.Println("")
	fmt.Println("Homepage: ", createLink("https://dotoca.net/pixu", "https://dotoca.net/pixu"))
	fmt.Println("Youtube:  ", createLink("https://youtube.com/@xvoland", "https://youtube.com/@xvoland"))
	fmt.Println("Donation: ", createLink("https://paypal.me/xvoland", "https://paypal.me/xvoland"))
	fmt.Printf("Copyright © %d, Vitalii Tereshchuk | URL: %s\n",
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
		{"PIXU_WIDTH", func(v string) { if n, err := strconv.Atoi(v); err == nil { width = n } }},
		{"PIXU_HEIGHT", func(v string) { if n, err := strconv.Atoi(v); err == nil { height = n } }},
		{"PIXU_MODE", func(v string) { mode = v }},
		{"PIXU_INVERT", func(v string) {
			if v == "1" || v == "true" || v == "TRUE" {
				invert = true
			} else if v == "0" || v == "false" || v == "FALSE" {
				invert = false
			}
		}},
		{"PIXU_CHAR", func(v string) { char = v }},
		{"PIXU_ROTATE", func(v string) { if n, err := strconv.Atoi(v); err == nil { rotate = n } }},
	}

	flagToEnv := map[string]string{
		"width": "PIXU_WIDTH",
		"height": "PIXU_HEIGHT",
		"mode": "PIXU_MODE",
		"invert": "PIXU_INVERT",
		"char": "PIXU_CHAR",
		"rotate": "PIXU_ROTATE",
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

		displayW := termW
		var displayH int
		if imgW > 0 {
			displayH = termW * imgH / imgW / 2
		} else {
			displayH = termH - statusLinesTerminal
		}

		if imgW > 0 && displayH > termH-statusLinesTerminal {
		displayH = termH - statusLinesTerminal
			displayW = displayH * 2 * imgW / imgH
		}

		renderer := &ImageRenderer{
			width: displayW,
			height: displayH,
			mode: mode,
			invert: invert,
			char: char,
			rotate: rotate,
			asciiChars: asciiChars,
			dither: dither,
		}
		lines := renderer.renderTerminal(img)
		dispH := len(lines)

		fmt.Print("\033[1;0H")
		for _, line := range lines {
			fmt.Print("\r")
			fmt.Println(line)
		}

		fmt.Print("\r")
	fmt.Printf("\033[%dH\033[7m%s | %dx%d | %s | %d/%d\033[0m\n", 
			dispH+1, filepath.Base(files[currentIndex]), imgW, imgH, strings.ToUpper(mode), currentIndex+1, len(files))
		fmt.Printf("\033[%dH\033[33m←/→: prev/next | ESC/Ctrl+C: quit\033[0m\n", dispH+2)
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
	validModes := map[string]bool{"rgb": true, "grayscale": true, "256": true, "ascii": true, "tgp": true}
	if !validModes[mode] {
		log.Fatalf("Invalid mode: %q (must be rgb, grayscale, 256, ascii, or tgp)", mode)
	}

	if !isValidRotate(rotate) {
		log.Fatalf("Invalid rotate value: %d (must be 0, 90, 180, 270, or 360)", rotate)
	}

	if width < 0 || height < 0 {
		log.Fatalf("Invalid width or height: width=%d height=%d (must be >= 0)", width, height)
	}
	if width > maxWidth || height > maxHeight {
		log.Fatalf("Width or height too large: width=%d height=%d (max %d)", width, height, maxWidth)
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
			resp, err := http.Get(clipText)
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

// renderAndOutput renders the image and writes to output
func renderAndOutput(img image.Image) {
	if mode == "tgp" {
		termW, termH := getTerminalSize()
		imgW := img.Bounds().Dx()
		imgH := img.Bounds().Dy()
		tgpW, tgpH := calculateTGPSize(imgW, imgH, width, height, termW, termH, statusLinesTGP)
		printTGP(img, tgpW, tgpH, rotate, invert)
		return
	}

	if fit {
		if tw, th := getTerminalSize(); tw > 0 {
			width = tw
			height = th - statusLinesTerminal
		}
	}

	scaleWidth, scaleHeight := calculateSize(img, width, height, false)

	ascii := os.Getenv("PIXU_ASCII_CHARS")
	if ascii == "" {
		ascii = "@#%*+=-:. "
	}

	renderer := &ImageRenderer{
		width:      scaleWidth,
		height:     scaleHeight,
		mode:       mode,
		invert:     invert,
		char:       char,
		rotate:     rotate,
		asciiChars: ascii,
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

	for _, line := range renderer.renderTerminal(img) {
		fmt.Fprintln(outputWriter, line)
	}
}

func main() {

	rootCmd := &cobra.Command{
		Use:  "pixu",
		Args: cobra.ArbitraryArgs,
Run: func(cmd *cobra.Command, args []string) {
		if showVersion {
			fmt.Println("pixu", version)
			printCopyleft()
			return
		}

		validateFlags()

		if qr {
			showQRCode()
			return
		}

		if interactive && (mode == "" || mode == "rgb") {
			mode = "tgp"
		}

		if interactive {
			asciiChars := os.Getenv("PIXU_ASCII_CHARS")
			if asciiChars == "" {
				asciiChars = "@#%*+=-:. "
			}
			runInteractiveMode(args, mode, invert, rotate, char, width, height, dither, asciiChars)
			return
		}

		img := loadImage(args)
		if img == nil {
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
	rootCmd.Flags().StringVarP(&mode, "mode", "m", "rgb", "Mode: rgb/grayscale/ascii/tgp")
	rootCmd.Flags().BoolVarP(&invert, "invert", "i", false, "Invert colors")
	rootCmd.Flags().StringVarP(&char, "char", "c", "▀", "Block character to use")
	rootCmd.Flags().IntVarP(&rotate, "rotate", "r", 0, "Rotate: 0,90,180,270,360")
	rootCmd.Flags().BoolVarP(&fit, "fit", "f", false, "Fit to terminal size")
	rootCmd.Flags().BoolVarP(&dither, "dither", "d", false, "Apply Floyd-Steinberg dithering")
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "I", false, "Interactive mode with navigation and zoom")
	rootCmd.Flags().BoolVarP(&qr, "qr", "", false, "Show QR code for donation")
	rootCmd.Flags().StringVar(&input, "input", "", "Input file (use - for stdin)")
	rootCmd.Flags().BoolVarP(&paste, "paste", "p", false, "Read image from clipboard")
	rootCmd.Flags().StringVarP(&output, "output", "o", "", "Output file (save to file instead of stdout)")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "Show version and exit")

	// version command only (like git)
	var versionCmd = &cobra.Command{
		Use: "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("PIXU version", version)
			printCopyleft()
		},
	}
	rootCmd.AddCommand(versionCmd)

	// parse flags first so we can check which were explicitly set
	rootCmd.ParseFlags(os.Args[1:])
	applyEnvDefaults(rootCmd) // apply env defaults, but not overriding explicit CLI flags

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
