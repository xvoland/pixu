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

	"github.com/disintegration/imaging"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	width, height, rotate int
	mode                  string
	invert                bool
	char                  string
	version               = "0.4.1"
)

// ImageRenderer holds rendering options
type ImageRenderer struct {
	width, height int
	mode          string
	invert        bool
	char          string
	rotate        int
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

// printTGP prints image using Kitty graphics protocol
func printTGP(img image.Image, width, height int) {
	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, resized); err != nil {
		log.Fatalf("Failed to encode image: %v", err)
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	fmt.Printf("\033_Gf=100,t=d,w=%d,h=%d;%s\033\\", width, height, data)
}

// renderTerminal returns terminal representation lines
func (r *ImageRenderer) renderTerminal(img image.Image) []string {
	img = rotateImage(img, r.rotate)
	resized := imaging.Resize(img, r.width, r.height*2, imaging.Lanczos)
	bounds := resized.Bounds()
	lines := make([]string, 0, bounds.Dy()/2)

	asciiChars := "@#%*+=-:. "
	if r.mode == "ascii" && r.char != "" {
		asciiChars = r.char
	}

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
				index := int(avgGray / 255 * float64(len(asciiChars)-1))
				line += string([]rune(asciiChars)[index])
			case "256":
				line += fmt.Sprintf("\033[38;5;%d;48;5;%dm▀", rgbTo256(tr8, tg8, tb8), rgbTo256(br8, bg8, bb8))
			case "grayscale":
				grayTop := uint8(0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8))
				grayBottom := uint8(0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8))
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s", grayTop, grayTop, grayTop, grayBottom, grayBottom, grayBottom, r.char)
			default:
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s", tr8, tg8, tb8, br8, bg8, bb8, r.char)
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
	} else {
		return fmt.Sprintf("\033]8;;%s\033\\%s\033]8;;\033\\", url, text)
	}
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "pixu <image>",
		Short: "Render images in terminal",
		Args:  cobra.MinimumNArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			imgPath := args[0]
			img, err := imaging.Open(imgPath)
			if err != nil {
				log.Fatalf("Error loading image: %v", err)
			}

			scaleWidth, scaleHeight := calculateSize(img, width, height, mode == "tgp")

			if mode == "tgp" {
				printTGP(img, scaleWidth, scaleHeight)
				return
			}

			renderer := &ImageRenderer{
				width:  scaleWidth,
				height: scaleHeight,
				mode:   mode,
				invert: invert,
				char:   char,
				rotate: rotate,
			}

			for _, line := range renderer.renderTerminal(img) {
				fmt.Println(line)
			}
		},
	}

	// remove default -h help to free it for height
	rootCmd.SetHelpCommand(&cobra.Command{Hidden: true})
	rootCmd.Flags().Bool("help", false, "Show help") // keep --help

	rootCmd.Flags().IntVarP(&height, "height", "h", 0, "Height in characters")
	rootCmd.Flags().IntVarP(&width, "width", "w", 0, "Width in characters")
	rootCmd.Flags().StringVarP(&mode, "mode", "m", "rgb", "Mode: rgb/grayscale/ascii/tgp")
	rootCmd.Flags().BoolVarP(&invert, "invert", "i", false, "Invert colors")
	rootCmd.Flags().StringVarP(&char, "char", "c", "▀", "Block character to use")
	rootCmd.Flags().IntVarP(&rotate, "rotate", "r", 0, "Rotate: 90,180,270")
	rootCmd.Flags().BoolP("version", "v", false, "Show version")

	// handle version
	rootCmd.PreRun = func(cmd *cobra.Command, args []string) {
		v, _ := cmd.Flags().GetBool("version")
		if v {
			fmt.Println("\033[1;36mPIXU version", version, "\033[0m")
			fmt.Println("")
			fmt.Println("Homepage: ", createLink("https://dotoca.net/pixu", "https://dotoca.net/pixu"))
			fmt.Println("Donation: ", createLink("https://paypal.me/xvoland", "https://paypal.me/xvoland"))
			fmt.Println("Copyright © 2025, Vitalii Tereshchuk | URL:", createLink("https://dotoca.net", "DOTOCA.NET"), "| All rights reserved.")
			os.Exit(0)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
