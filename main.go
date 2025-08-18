package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"os"

	"github.com/disintegration/imaging"
	"golang.org/x/term"
)

type ImageRenderer struct {
	width  int
	height int
}

func (r *ImageRenderer) getBlockArtLines(img image.Image) []string {
	// Увеличиваем высоту в 2 раза, чтобы использовать полублоки
	resized := imaging.Resize(img, r.width, r.height*2, imaging.Lanczos)
	bounds := resized.Bounds()

	lines := make([]string, 0, bounds.Dy()/2)

	for y := bounds.Min.Y; y < bounds.Max.Y; y += 2 {
		line := ""

		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			topColor := resized.At(x, y)
			var bottomColor color.Color
			if y+1 < bounds.Max.Y {
				bottomColor = resized.At(x, y+1)
			} else {
				bottomColor = topColor
			}

			tr, tg, tb, _ := topColor.RGBA()
			br, bg, bb, _ := bottomColor.RGBA()

			// Конвертируем в 8-бит
			tr8, tg8, tb8 := uint8(tr>>8), uint8(tg>>8), uint8(tb>>8)
			br8, bg8, bb8 := uint8(br>>8), uint8(bg>>8), uint8(bb>>8)

			// Верхний полублок '▀': fg = верхний цвет, bg = нижний цвет
			line += fmt.Sprintf("\033[38;2;%d;%d;%d;48;2;%d;%d;%dm▀", tr8, tg8, tb8, br8, bg8, bb8)
		}

		line += "\033[0m" // сброс цвета
		lines = append(lines, line)
	}

	return lines
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: go run main.go <путь к изображению>")
		return
	}

	imgPath := os.Args[1]

	img, err := imaging.Open(imgPath)
	if err != nil {
		log.Fatalf("Ошибка загрузки изображения: %v", err)
	}

	// Размер терминала
	termWidth, termHeight, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil {
		termWidth, termHeight = 80, 40
	}

	maxWidthBlocks := termWidth
	maxHeightBlocks := int(float64(termHeight) * 2) // *2 для полублоков

	imgWidth := img.Bounds().Dx()
	imgHeight := img.Bounds().Dy()

	// Пропорциональное масштабирование
	scaleWidth := maxWidthBlocks
	scaleHeight := int(math.Round(float64(imgHeight) * float64(scaleWidth) / float64(imgWidth)))

	if scaleHeight > maxHeightBlocks {
		scaleHeight = maxHeightBlocks
		scaleWidth = int(math.Round(float64(imgWidth) * float64(scaleHeight) / float64(imgHeight)))
	}

	if scaleWidth < 1 {
		scaleWidth = 1
	}
	if scaleHeight < 1 {
		scaleHeight = 1
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
