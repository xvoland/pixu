package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"
	"strconv"

	"github.com/disintegration/imaging"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	width, height, rotate int
	mode                  string
	invert                bool
	char                  string
	version               = "0.5.0"
	showVersion           bool
)

// ImageRenderer holds rendering options
type ImageRenderer struct {
	width, height int
	mode          string
	invert        bool
	char          string
	rotate        int
	asciiChars    string
}

// rgbTo256 converts RGB to 256-color terminal code
func rgbTo256(r, g, b uint8) int {
	r6 := int(float64(r) * 5 / 255)
	g6 := int(float64(g) * 5 / 255)
	b6 := int(float64(b) * 5 / 255)
	return 16 + 36*r6 + 6*g6 + b6
}

// calculateSize computes width/height automatically
func calculateSize(img image.Image, width, height int, isTGP bool) (int, int) {
	imgW := img.Bounds().Dx()
	imgH := img.Bounds().Dy()

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

	// Rotate the image according to the specified angle
	switch rotate {
	case 90:
		img = imaging.Rotate90(img)
	case 180:
		img = imaging.Rotate180(img)
	case 270:
		img = imaging.Rotate270(img)
	}

	// Invert the image colors if invert flag is true
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
		// iTerm2 inline image protocol
		// \033]1337;File=... sends image inline in iTerm2
		fmt.Printf("\033]1337;File=name=inline.png;width=auto;height=auto;inline=1:%s\a\n", data)
	default:
		// Kitty / Ghostty graphics protocol
		// Split base64 data into chunks to comply with terminal buffer limits
		for i := 0; i < len(data); i += chunkSize {
			end := i + chunkSize
			if end > len(data) {
				end = len(data)
			}
			chunk := data[i:end]

			// more=1 indicates that there are more chunks to follow
			more := 0
			if end < len(data) {
				more = 1
			}

			if i == 0 {
				// First chunk includes full parameters (action, format, type)
				fmt.Printf("\033_Ga=T,f=100,t=d,m=%d;%s\033\\", more, chunk)
			} else {
				// Subsequent chunks only include 'more' parameter
				fmt.Printf("\033_Gm=%d;%s\033\\", more, chunk)
			}
		}
		// Print newline to clean up terminal output (avoid leftover characters like '%')
		fmt.Print("\n")
	}
}

// renderTerminal returns terminal representation lines
func (r *ImageRenderer) renderTerminal(img image.Image) []string {
	img = rotateImage(img, r.rotate)
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

			switch r.mode {
			case "ascii":
				grayTop := 0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8)
				grayBottom := 0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8)
				avgGray := (grayTop + grayBottom) / 2

				// asciiChars := "@#%*+=-:. "
				chars := r.asciiChars
				if r.char != "" {
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
				line += fmt.Sprintf("\033[38;5;%d;48;5;%dm▀",
					rgbTo256(tr8, tg8, tb8),
					rgbTo256(br8, bg8, bb8))
			case "grayscale":
				grayTop := uint8(0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8))
				grayBottom := uint8(0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8))
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s",
					grayTop, grayTop, grayTop,
					grayBottom, grayBottom, grayBottom,
					r.char)
			default: // rgb
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s",
					tr8, tg8, tb8,
					br8, bg8, bb8,
					r.char)
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

func main() {
	rootCmd := &cobra.Command{
		Use:   "pixu",
		Short: "\033[1;36mPIXU: ANSI and TGP render images in terminal\033[0m",
		Args:  cobra.MaximumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if showVersion {
				fmt.Println(version)
				return
			}

			if len(args) == 0 {
				cmd.Help()
				return
			}

			imgPath := args[0]
			img, err := imaging.Open(imgPath, imaging.AutoOrientation(true))
			if err != nil {
				log.Fatalf("Error loading image: %v", err)
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
	rootCmd.PersistentFlags().BoolVarP(&showVersion, "version", "v", false, "Show version and exit")

	applyEnvDefaults() // apply default values from environment variables

	// version command only (like git)
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Show version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("\033[1;36mPIXU version", version, "\033[0m")
			fmt.Println("")
			fmt.Println("Homepage: ", createLink("https://dotoca.net/pixu", "https://dotoca.net/pixu"))
			fmt.Println("Donation: ", createLink("https://paypal.me/xvoland", "https://paypal.me/xvoland"))
			fmt.Println("Copyright © 2025, Vitalii Tereshchuk | URL:",
				createLink("https://dotoca.net", "DOTOCA.NET"))
		},
	}
	rootCmd.AddCommand(versionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
