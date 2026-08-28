package main

import (
	"image"
	"os"
	"testing"
)

func TestRgbTo256GrayscaleRamp(t *testing.T) {
	// near-gray should map into the 232-255 ramp
	if c := rgbTo256(100, 100, 100); c < 232 || c > 255 {
		t.Errorf("expected grayscale ramp 232-255, got %d", c)
	}
	// pure black -> 232, pure white -> 255
	if got := rgbTo256(0, 0, 0); got != 232 {
		t.Errorf("black should be 232, got %d", got)
	}
	if got := rgbTo256(255, 255, 255); got != 255 {
		t.Errorf("white should be 255, got %d", got)
	}
}

func TestRgbTo256ColorCube(t *testing.T) {
	// a clearly colored value should land in the 16-231 cube range
	if c := rgbTo256(255, 0, 0); c < 16 || c > 231 {
		t.Errorf("red should be in 16-231 cube, got %d", c)
	}
}

func TestIsValidRotate(t *testing.T) {
	for _, v := range []int{0, 90, 180, 270, 360} {
		if !isValidRotate(v) {
			t.Errorf("%d should be valid", v)
		}
	}
	for _, v := range []int{45, 100, -90, 450} {
		if isValidRotate(v) {
			t.Errorf("%d should be invalid", v)
		}
	}
}

func TestCalculateSizeTerminal(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100)) // 2:1
	w, h := calculateSize(img, 80, 0, false)
	if w != 80 {
		t.Errorf("width should stay 80, got %d", w)
	}
	if h != 20 { // 80 * 100/200 / 2 = 20
		t.Errorf("height should be 20, got %d", h)
	}
}

func TestCalculateSizeTGP(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 200, 100))
	w, h := calculateSize(img, 80, 0, true)
	if w != 80 {
		t.Errorf("width should stay 80, got %d", w)
	}
	if h != 40 { // 80 * 100/200 = 40
		t.Errorf("height should be 40, got %d", h)
	}
}

func TestCalculateTGPSizeClamp(t *testing.T) {
	// a huge explicit width must be clamped to the terminal pixel size
	w, _ := calculateTGPSize(100, 100, 100000, 0, 80, 24, statusLinesTGP)
	if w > 80*cellWidthPxDefault {
		t.Errorf("width should be clamped to terminal (%d), got %d", 80*cellWidthPxDefault, w)
	}
}

func TestRotateImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 20))
	r := rotateImage(img, 90)
	if r.Bounds().Dx() != 20 || r.Bounds().Dy() != 10 {
		t.Errorf("after 90deg rotate expected 20x10, got %dx%d", r.Bounds().Dx(), r.Bounds().Dy())
	}
}

func TestSixelSupported(t *testing.T) {
	old := os.Getenv("TERM_PROGRAM")
	defer os.Setenv("TERM_PROGRAM", old)

	os.Setenv("TERM_PROGRAM", "iTerm.app")
	if sixelSupported() {
		t.Errorf("iTerm.app should not support sixel")
	}
	os.Setenv("TERM_PROGRAM", "ghostty")
	if !sixelSupported() {
		t.Errorf("ghostty should support sixel")
	}
}
