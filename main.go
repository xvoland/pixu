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
}

func (r *ImageRenderer) getBlockArtLines(img image.Image) []string {
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

			line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", tr8, tg8, tb8, br8, bg8, bb8)
		}
		line += "\033[0m"
		lines = append(lines, line)
	}

	return lines
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: go run main.go <путь к изображению> [--width=<число>] [--height=<число>]")
		return
	}

	imgPath := os.Args[1]

	// Флаги начинаем парсить со второго аргумента
	widthPtr := flag.Int("width", 0, "Ширина изображения в символах")
	heightPtr := flag.Int("height", 0, "Высота изображения в полублоках")
	flag.CommandLine.Parse(os.Args[2:])

	img, err := imaging.Open(imgPath)
	if err != nil {
		log.Fatalf("Ошибка загрузки изображения: %v", err)
	}

	imgWidth := img.Bounds().Dx()
	imgHeight := img.Bounds().Dy()

	scaleWidth := *widthPtr
	scaleHeight := *heightPtr

	// Если оба нуля, ставим стандартные значения
	if scaleWidth == 0 && scaleHeight == 0 {
		scaleWidth = 40
		scaleHeight = 20
	}

	// Авторасчёт пропорций
	if scaleWidth > 0 && scaleHeight == 0 {
		scaleHeight = int(math.Round(float64(imgHeight) * float64(scaleWidth) / float64(imgWidth) / 2))
	} else if scaleHeight > 0 && scaleWidth == 0 {
		scaleWidth = int(math.Round(float64(imgWidth) * float64(scaleHeight*2) / float64(imgHeight)))
	}

	renderer := &ImageRenderer{
		width:  scaleWidth,
		height: scaleHeight,
	}

	lines := renderer.getBlockArtLines(img)

	for _, line := range lines {
		fmt.Println(line)
	}
}
