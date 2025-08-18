package main

import (
	"flag"
	"fmt"
	"image"
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
}

func rgbTo256(r, g, b uint8) int {
	r6 := int(float64(r) * 5 / 255)
	g6 := int(float64(g) * 5 / 255)
	b6 := int(float64(b) * 5 / 255)
	return 16 + 36*r6 + 6*g6 + b6
}

func (r *ImageRenderer) getBlockArtLines(img image.Image) []string {
	resized := imaging.Resize(img, r.width, r.height*2, imaging.Lanczos)
	bounds := resized.Bounds()

	lines := make([]string, 0, bounds.Dy()/2)
	asciiChars := "@#%*+=-:. "
	if r.mode == "ascii" && r.char != "" {
		asciiChars = r.char // use user-defined ASCII characters
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
				// Convert user-provided string to rune slice for proper indexing
				asciiRunes := []rune(asciiChars)
				grayTop := 0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8)
				grayBottom := 0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8)
				avgGray := (grayTop + grayBottom) / 2
				index := int(avgGray / 255 * float64(len(asciiRunes)-1))
				line += string(asciiRunes[index])
			case "asc2":
				// Simple grayscale to ascii mapping
				asciiChars := "@#%*+=-:. "
				tGray := 0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8)
				bGray := 0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8)
				tChar := asciiChars[int(tGray/255*float64(len(asciiChars)-1))]
				bChar := asciiChars[int(bGray/255*float64(len(asciiChars)-1))]
				line += fmt.Sprintf("%c%c", tChar, bChar)
			case "256":
				line += fmt.Sprintf("\033[38;5;%d;48;5;%dm▀", rgbTo256(tr8, tg8, tb8), rgbTo256(br8, bg8, bb8))
			case "grayscale":
				grayTop := uint8(0.299*float64(tr8) + 0.587*float64(tg8) + 0.114*float64(tb8))
				grayBottom := uint8(0.299*float64(br8) + 0.587*float64(bg8) + 0.114*float64(bb8))
				line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm%s", grayTop, grayTop, grayTop, grayBottom, grayBottom, grayBottom, r.char)
			default: // rgb
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

func main() {
	// Ensure first argument is image
	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <image> [flags]")
		return
	}
	imgPath := os.Args[1]

	// Parse flags from remaining args
	widthPtr := flag.Int("width", 0, "Width in characters")
	heightPtr := flag.Int("height", 0, "Height in characters")
	modePtr := flag.String("mode", "rgb", "Mode: rgb/grayscale/ascii")
	invertPtr := flag.Bool("invert", false, "Invert colors")
	charPtr := flag.String("char", "▀", "Block character to use")
	flag.CommandLine.Parse(os.Args[2:])

	img, err := imaging.Open(imgPath)
	if err != nil {
		log.Fatalf("Error loading image: %v", err)
	}

	imgWidth := img.Bounds().Dx()
	imgHeight := img.Bounds().Dy()

	scaleWidth := *widthPtr
	scaleHeight := *heightPtr

	// Auto calculate width/height if not set
	if scaleWidth > 0 && scaleHeight == 0 {
		scaleHeight = int(math.Round(float64(imgHeight) * float64(scaleWidth) / float64(imgWidth) / 2))
	} else if scaleHeight > 0 && scaleWidth == 0 {
		scaleWidth = int(math.Round(float64(imgWidth) * float64(scaleHeight*2) / float64(imgHeight)))
	} else if scaleHeight == 0 && scaleWidth == 0 {
		scaleWidth = 40
		scaleHeight = int(math.Round(float64(imgHeight) * float64(scaleWidth) / float64(imgWidth) / 2))
	}

	renderer := &ImageRenderer{
		width:  scaleWidth,
		height: scaleHeight,
		mode:   *modePtr,
		invert: *invertPtr,
		char:   *charPtr,
	}

	lines := renderer.getBlockArtLines(img)

	for _, line := range lines {
		fmt.Println(line)
	}
}
