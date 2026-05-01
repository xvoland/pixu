//go:build simple

package main

import (
	"bytes"
	"encoding/base64"
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"math"
	"os"

	"github.com/disintegration/imaging"
)

type ImageRenderer struct {
	width  int
	height int
	mode   string
	invert bool
	char   string
	rotate int
}

// rgbTo256 converts RGB values to 256-color terminal code
func rgbTo256(r, g, b uint8) int {
	r6 := int(float64(r) * 5 / 255)
	g6 := int(float64(g) * 5 / 255)
	b6 := int(float64(b) * 5 / 255)
	return 16 + 36*r6 + 6*g6 + b6
}

// calculateSize computes missing width or height automatically
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

// rotateImage rotates the image according to specified degrees
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

// printTGP prints the image using Kitty graphics protocol
func printTGP(img image.Image, width, height int) {
	resized := imaging.Resize(img, width, height, imaging.Lanczos)
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, resized); err != nil {
		log.Fatalf("Failed to encode image: %v", err)
	}
	data := base64.StdEncoding.EncodeToString(buf.Bytes())
	fmt.Printf("\033_Gf=100,t=d,w=%d,h=%d;%s\033\\", width, height, data)
}

// renderTerminal generates terminal representation (ASCII/256/Grayscale/RGB)
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
			line += "\033[0m" // reset colors
		}
		lines = append(lines, line)
	}

	return lines
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <image> [flags]")
		return
	}
	imgPath := os.Args[1]

	widthPtr := flag.Int("width", 0, "Width in characters")
	heightPtr := flag.Int("height", 0, "Height in characters")
	modePtr := flag.String("mode", "rgb", "Mode: rgb/grayscale/ascii/tgp")
	invertPtr := flag.Bool("invert", false, "Invert colors")
	charPtr := flag.String("char", "▀", "Block character to use")
	rotateDegree := flag.Int("rotate", 0, "Rotate degree: 90, 180, 270")
	flag.CommandLine.Parse(os.Args[2:])

	img, err := imaging.Open(imgPath)
	if err != nil {
		log.Fatalf("Error loading image: %v", err)
	}

	scaleWidth, scaleHeight := calculateSize(img, *widthPtr, *heightPtr, *modePtr == "tgp")

	if *modePtr == "tgp" {
		printTGP(img, scaleWidth, scaleHeight)
		return
	}

	renderer := &ImageRenderer{
		width:  scaleWidth,
		height: scaleHeight,
		mode:   *modePtr,
		invert: *invertPtr,
		char:   *charPtr,
		rotate: *rotateDegree,
	}

	for _, line := range renderer.renderTerminal(img) {
		fmt.Println(line)
	}
}
