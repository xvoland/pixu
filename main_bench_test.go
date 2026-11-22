package main

import (
	"image"
	"image/color"
	"testing"
)

// benchImage returns a deterministic 400x400 RGBA image so benchmarks stay
// stable across runs and do not depend on the filesystem.
func benchImage() *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 400, 400))
	for y := 0; y < 400; y++ {
		for x := 0; x < 400; x++ {
			img.Set(x, y, color.RGBA{
				R: uint8((x * 7) % 256),
				G: uint8((y * 13) % 256),
				B: uint8((x + y) % 256),
				A: 255,
			})
		}
	}
	return img
}

func BenchmarkRenderTerminalRGB(b *testing.B) {
	src := benchImage()
	r := &ImageRenderer{width: 100, height: 40, mode: "rgb"}
	b.ReportAllocs()
	for b.Loop() {
		_ = r.renderTerminal(src)
	}
}

func BenchmarkRenderTerminal256(b *testing.B) {
	src := benchImage()
	r := &ImageRenderer{width: 100, height: 40, mode: "256"}
	b.ReportAllocs()
	for b.Loop() {
		_ = r.renderTerminal(src)
	}
}

func BenchmarkRenderTerminalASCII(b *testing.B) {
	src := benchImage()
	r := &ImageRenderer{width: 100, height: 40, mode: "ascii", asciiChars: "@#%*+=-:. "}
	b.ReportAllocs()
	for b.Loop() {
		_ = r.renderTerminal(src)
	}
}

func BenchmarkRenderTerminalGrayscale(b *testing.B) {
	src := benchImage()
	r := &ImageRenderer{width: 100, height: 40, mode: "grayscale"}
	b.ReportAllocs()
	for b.Loop() {
		_ = r.renderTerminal(src)
	}
}

func BenchmarkFloydSteinberg(b *testing.B) {
	src := benchImage()
	b.ReportAllocs()
	for b.Loop() {
		_ = applyFloydSteinberg(src)
	}
}

func BenchmarkRgbTo256(b *testing.B) {
	i := 0
	b.ReportAllocs()
	for b.Loop() {
		_ = rgbTo256(uint8(i), uint8(i*3), uint8(i*7))
		i++
	}
}

// BenchmarkPrepareImage measures the resize path used by the TGP and Sixel
// protocols: rotate/invert an image and resample it with Lanczos.
func BenchmarkPrepareImage(b *testing.B) {
	src := benchImage()
	b.ReportAllocs()
	for b.Loop() {
		_, _ = prepareImage(src, 800, 600, 90, false)
	}
}
