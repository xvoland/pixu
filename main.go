package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/disintegration/imaging"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

const DEFAULT_VERSION = "x.x.x"

var qrCodeText = "Scan to support PIXU!\nhttps://paypal.me/xvoland\n"

var buildSource = "local"

var qrCodeBase64 = ""

var qrCodeData []byte

func init() {
	if qrCodeBase64 != "" {
		qrCodeData, _ = base64.StdEncoding.DecodeString(qrCodeBase64)
	}
}

var (
	width, height, rotate int
	mode                  string
	invert                bool
	char                  string
	version               = DEFAULT_VERSION
	showVersion           bool
	fit                   bool
	dither                bool
	interactive           bool
	qr                    bool
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

// rgbTo256 converts RGB to 256-color terminal code
func rgbTo256(r, g, b uint8) int {
	r6 := int(float64(r) * 5 / 255)
	g6 := int(float64(g) * 5 / 255)
	b6 := int(float64(b) * 5 / 255)
	return 16 + 36*r6 + 6*g6 + b6
}

func applyFloydSteinberg(img image.Image) image.Image {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

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
			}
			if oldG > 255 {
				oldG = 255
			}
			if oldB > 255 {
				oldB = 255
			}

			newR := uint8(oldR)
			newG := uint8(oldG)
			newB := uint8(oldB)

			newImg.Set(x+bounds.Min.X, y+bounds.Min.Y, color.RGBA{
				R: newR, G: newG, B: newB, A: uint8(a >> 8),
			})

			errR[y][x] = oldR - float64(newR)
			errG[y][x] = oldG - float64(newG)
			errB[y][x] = oldB - float64(newB)

			if x+1 < w {
				errR[y][x+1] += errR[y][x] * 7 / 16
				errG[y][x+1] += errG[y][x] * 7 / 16
				errB[y][x+1] += errB[y][x] * 7 / 16
			}
			if y+1 < h {
				if x > 0 {
					errR[y+1][x-1] += errR[y][x] * 3 / 16
					errG[y+1][x-1] += errG[y][x] * 3 / 16
					errB[y+1][x-1] += errB[y][x] * 3 / 16
				}
				errR[y+1][x] += errR[y][x] * 5 / 16
				errG[y+1][x] += errG[y][x] * 5 / 16
				errB[y+1][x] += errB[y][x] * 5 / 16
				if x+1 < w {
					errR[y+1][x+1] += errR[y][x] * 1 / 16
					errG[y+1][x+1] += errG[y][x] * 1 / 16
					errB[y+1][x+1] += errB[y][x] * 1 / 16
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
	if !term.IsTerminal(fd) {
		if tty, err := os.Open("/dev/tty"); err == nil {
			fd = int(tty.Fd())
			tty.Close()
		}
	}
	w, h, err := term.GetSize(fd)
	if err != nil {
		return 80, 24
	}
	return w, h
}

// rotateImage rotates the image by degrees
func rotateImage(img image.Image, degrees int) image.Image {
	switch degrees {
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
		line := ""
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
				line += string(runes[index])
			case "256":
				line += fmt.Sprintf("\033[38;5;%d;48;5;%dm%s",
					rgbTo256(tr8, tg8, tb8),
					rgbTo256(br8, bg8, bb8),
					charToUse)
			case "grayscale":
				grayTop := uint8(0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8))
				grayBottom := uint8(0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8))
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s",
					grayTop, grayTop, grayTop,
					grayBottom, grayBottom, grayBottom,
					charToUse)
			default: // rgb
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s",
					tr8, tg8, tb8,
					br8, bg8, bb8,
					charToUse)
			}
		}

		if r.mode != "ascii" {
			line += "\033[0m"
		}
		lines = append(lines, line)
	}

	return lines
}

func createLink(url, text string) string {
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return fmt.Sprintf("%s", url)
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

// apply environment defaults
func applyEnvDefaults() {
	var envDefaults = map[string]interface{}{
		"PIXU_WIDTH":  &width,
		"PIXU_HEIGHT": &height,
		"PIXU_MODE":   &mode,
		"PIXU_INVERT": &invert,
		"PIXU_CHAR":   &char,
		"PIXU_ROTATE": &rotate,
	}

	for env, ptr := range envDefaults {
		val := os.Getenv(env)
		if val == "" {
			continue
		}

		switch p := ptr.(type) {
		case *int:
			if v, err := strconv.Atoi(val); err == nil {
				*p = v
			}
		case *string:
			*p = val
		case *bool:
			if val == "1" || val == "true" || val == "TRUE" {
				*p = true
			} else if val == "0" || val == "false" || val == "FALSE" {
				*p = false
			}
		}
	}
}

func runInteractiveMode(args []string, mode string, invert bool, rotate int, char string, width int, height int) {
	imageExtensions := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true, ".gif": true, ".bmp": true, ".webp": true,
	}

	files := args
	if len(files) == 1 {
		if isDir, _ := pathExists(files[0]); isDir {
			dirPath := files[0]
			entries, _ := os.ReadDir(dirPath)
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

		fmt.Print("\033[2J")

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
			displayW := width
			displayH := height

			termPixelW := termW * 10
			termPixelH := (termH - 6) * 20

			if displayW == 0 && displayH == 0 {
				displayW = termPixelW
				displayH = int(math.Round(float64(imgH) * float64(displayW) / float64(imgW)))
				if displayH > termPixelH {
					displayH = termPixelH
					displayW = int(math.Round(float64(imgW) * float64(displayH) / float64(imgH)))
				}
			} else if displayW > 0 && displayH == 0 && imgW > 0 {
				displayH = int(math.Round(float64(imgH) * float64(displayW) / float64(imgW)))
			} else if displayH > 0 && displayW == 0 && imgH > 0 {
				displayW = int(math.Round(float64(imgW) * float64(displayH) / float64(imgH)))
			}

			resized := imaging.Resize(img, displayW, displayH, imaging.Lanczos)
			printTGPKittyFromImage(resized)

			termRowH := displayH/20 + 3
			fmt.Printf("\r\033[%dH\033[7m%s | %dx%d | TGP | %d/%d\033[0m\n",
				termRowH, filepath.Base(files[currentIndex]), imgW, imgH, currentIndex+1, len(files))
			fmt.Printf("\033[%dH\033[33m←/→: prev/next | ESC/Ctrl+C: quit\033[0m\n", termRowH+1)
			return
		}

		displayW := termW
		displayH := termW * imgH / imgW / 2

		if displayH > termH-4 {
			displayH = termH - 4
			displayW = displayH * 2 * imgW / imgH
		}

		resized := imaging.Resize(img, displayW, displayH, imaging.Lanczos)
		bounds = resized.Bounds()
		dispH := bounds.Dy()

		for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
			fmt.Print("\r")
			line := ""
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

				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm▀\033[0m",
					tr8, tg8, tb8, br8, bg8, bb8)
			}
			fmt.Print(line)
			fmt.Println()
		}

		fmt.Print("\r")
		fmt.Printf("\033[%dH\033[7m%s | %dx%d | ASCII | %d/%d\033[0m\n",
			dispH+3, filepath.Base(files[currentIndex]), imgW, imgH, currentIndex+1, len(files))
		fmt.Printf("\033[%dH\033[33m←/→: prev/next | ESC/Ctrl+C: quit\033[0m\n", dispH+4)
	}

	showImage()

	tty, err := os.Open("/dev/tty")
	if err != nil {
		fmt.Println("Error: cannot open /dev/tty for input")
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
		png.Encode(buf, resized)
		data := base64.StdEncoding.EncodeToString(buf.Bytes())
		fmt.Printf("\033]1337;File=name=qr.png;width=auto;height=auto;inline=1:%s\a\n", data)
	default:
		printTGPKittyFromImage(resized)
	}

	if qrCodeText != "" {
		fmt.Println()
		fmt.Println(qrCodeText)
	}
}

func printTGPKittyFromImage(img image.Image) {
	fmt.Print("\033[3;0H")

	buf := new(bytes.Buffer)
	png.Encode(buf, img)
	data := base64.StdEncoding.EncodeToString(buf.Bytes())

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

func main() {

	rootCmd := &cobra.Command{
		Use:   "pixu",
		Short: "PIXU: ANSI and TGP render images in terminal",
		Args:  cobra.ArbitraryArgs,
		Run: func(cmd *cobra.Command, args []string) {
			if showVersion {
				fmt.Println("pixu", version)
				printCopyleft()
				return
			}

			if qr {
				showQRCode()
				return
			}

			if len(args) == 0 {
				cmd.Help()
				return
			}

			if interactive && (mode == "" || mode == "rgb") {
				mode = "tgp"
			}

			if interactive {
				runInteractiveMode(args, mode, invert, rotate, char, width, height)
				return
			}

			imgPath := args[0]
			img, err := imaging.Open(imgPath, imaging.AutoOrientation(true))
			if err != nil {
				log.Fatalf("Error loading image: %v", err)
			}

			if rotate != 0 && rotate != 90 && rotate != 180 && rotate != 270 {
				log.Fatalf("Invalid rotate value: %d (must be 0, 90, 180, or 270)", rotate)
			}

			if fit || (mode == "tgp" && width == 0 && height == 0) {
				if tw, th := getTerminalSize(); tw > 0 {
					width = tw
					height = th - 4
				}
			}

			scaleWidth, scaleHeight := calculateSize(img, width, height, mode == "tgp")

			if mode == "tgp" {
				printTGP(img, scaleWidth, scaleHeight, rotate, invert)
				return
			}

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

			for _, line := range renderer.renderTerminal(img) {
				fmt.Println(line)
			}
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
	rootCmd.Flags().IntVarP(&rotate, "rotate", "r", 0, "Rotate: 90,180,270")
	rootCmd.Flags().BoolVarP(&fit, "fit", "f", false, "Fit to terminal size")
	rootCmd.Flags().BoolVarP(&dither, "dither", "d", false, "Apply Floyd-Steinberg dithering")
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "I", false, "Interactive mode with navigation and zoom")
	rootCmd.Flags().BoolVarP(&qr, "qr", "", false, "Show QR code for donation")
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "Show version and exit")

	applyEnvDefaults() // apply default values from environment variables

	// version command only (like git)
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("PIXU version", version)
			printCopyleft()
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
