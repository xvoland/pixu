package main

import (
	"errors"
	"image"
	"image/color"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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
	// An extreme explicit width must be bounded by the absolute maximum, but it
	// is no longer clamped to the terminal (so fit/scale/width can enlarge).
	w, _ := calculateTGPSize(100, 100, 100000, 0, 80, 24, statusLinesTGP)
	if w != maxWidth {
		t.Errorf("width should be clamped to maxWidth (%d), got %d", maxWidth, w)
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

func TestResolveMode(t *testing.T) {
	keys := []string{"TERM_PROGRAM", "TERM", "KITTY_WINDOW_ID", "COLORTERM"}
	orig := make(map[string]string, len(keys))
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			orig[k] = v
		}
	}
	defer func() {
		for _, k := range keys {
			if v, ok := orig[k]; ok {
				os.Setenv(k, v)
			} else {
				os.Unsetenv(k)
			}
		}
	}()

	set := func(termProg, term, kitty, color string) {
		os.Setenv("TERM_PROGRAM", termProg)
		os.Setenv("TERM", term)
		if kitty == "" {
			os.Unsetenv("KITTY_WINDOW_ID")
		} else {
			os.Setenv("KITTY_WINDOW_ID", kitty)
		}
		os.Setenv("COLORTERM", color)
	}

	cases := []struct {
		name, termProg, term, kitty, color, want string
	}{
		{"iterm", "iTerm.app", "xterm-256color", "", "", "tgp"},
		{"ghostty", "ghostty", "xterm-256color", "", "", "tgp"},
		{"wezterm", "WezTerm", "xterm-256color", "", "", "tgp"},
		{"kitty-term", "", "xterm-kitty", "", "", "tgp"},
		{"kitty-env", "", "xterm", "1", "", "tgp"},
		{"foot-sixel", "foot", "foot", "", "", "sixel"},
		{"xterm-sixel", "xterm", "xterm", "", "", "sixel"},
		{"apple-truecolor", "Apple_Terminal", "xterm-256color", "", "truecolor", "rgb"},
		{"terminal-256", "Terminal", "xterm-256color", "", "", "256"},
		{"terminal-basic", "Terminal", "xterm", "", "", "grayscale"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			set(c.termProg, c.term, c.kitty, c.color)
			if got := resolveMode("auto"); got != c.want {
				t.Errorf("resolveMode(auto) = %q, want %q", got, c.want)
			}
		})
	}

	// non-auto modes are returned unchanged
	if got := resolveMode("sixel"); got != "sixel" {
		t.Errorf("resolveMode(sixel) should be unchanged, got %q", got)
	}
}

// newEnvCmd builds a command with the same flags (bound to the same globals)
// that applyEnvDefaults consults, avoiding a dependency on the package-private
// rootCmd.
func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{}
	cmd.Flags().IntVarP(&width, "width", "w", 0, "")
	cmd.Flags().Float64VarP(&scale, "scale", "S", 1.0, "")
	cmd.Flags().StringVarP(&mode, "mode", "m", "rgb", "")
	return cmd
}

// TestEnvPrecedence verifies the documented priority: CLI flag > env > default.
// It builds a command with the same flags (bound to the same globals) that
// applyEnvDefaults consults, avoiding a dependency on the package-private rootCmd.
func TestEnvPrecedence(t *testing.T) {
	reset := func() {
		width = 0
		scale = 1.0
		mode = "rgb"
	}
	cases := []struct {
		name, envKey, envVal, args string
		get                        func() interface{}
		want                       interface{}
	}{
		{"width-flag-wins", "PIXU_WIDTH", "111", "-w 999", func() interface{} { return width }, 999},
		{"width-env-only", "PIXU_WIDTH", "222", "", func() interface{} { return width }, 222},
		{"scale-flag-wins", "PIXU_SCALE", "0.5", "-S 3", func() interface{} { return scale }, 3.0},
		{"scale-env-only", "PIXU_SCALE", "0.5", "", func() interface{} { return scale }, 0.5},
		{"mode-flag-wins", "PIXU_MODE", "auto", "-m sixel", func() interface{} { return mode }, "sixel"},
		{"mode-env-only", "PIXU_MODE", "auto", "", func() interface{} { return mode }, "auto"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reset()
			os.Setenv(c.envKey, c.envVal)
			defer os.Unsetenv(c.envKey)

			cmd := newEnvCmd()
			cmd.ParseFlags(strings.Fields(c.args))
			applyEnvDefaults(cmd)

			if got := c.get(); got != c.want {
				t.Errorf("got %v, want %v (env=%s=%q args=%q)", got, c.want, c.envKey, c.envVal, c.args)
			}
		})
	}
}

// TestEnvAbsentKeepsDefault verifies that when no PIXU_* variables are set,
// applyEnvDefaults leaves the defaults untouched (absent env is a no-op).
func TestEnvAbsentKeepsDefault(t *testing.T) {
	for _, k := range []string{"PIXU_WIDTH", "PIXU_SCALE", "PIXU_MODE", "PIXU_INVERT", "PIXU_ROTATE"} {
		os.Unsetenv(k)
	}

	width = 0
	scale = 1.0
	mode = "rgb"
	invert = false
	rotate = 0

	cmd := newEnvCmd()
	cmd.ParseFlags([]string{})
	applyEnvDefaults(cmd)

	if width != 0 || scale != 1.0 || mode != "rgb" || invert || rotate != 0 {
		t.Errorf("absent env should keep defaults: width=%d scale=%v mode=%q invert=%v rotate=%d",
			width, scale, mode, invert, rotate)
	}
}

// TestFitToTerminal verifies that --fit preserves the image aspect ratio and
// aligns by the requested axis (height by default / "H", width for "W").
func TestFitToTerminal(t *testing.T) {
	const (
		termW = 80
		termH = 24
	)

	// Text modes render each row as two vertical pixels, so the effective canvas
	// aspect is width / (2*height). Both axes must keep it equal to imgW/imgH.
	checkText := func(axis string, imgW, imgH int) {
		w, h := fitToTerminal(imgW, imgH, termW, termH, "rgb", axis)
		got := float64(w) / float64(2*h)
		want := float64(imgW) / float64(imgH)
		// Terminal rows are quantized to whole cells (each = 2 vertical pixels),
		// so a residual error of ~0.1 from rounding is expected and not distortion.
		if math.Abs(got-want) > 0.1 {
			t.Errorf("text fit %q %dx%d: canvas aspect %.3f, want ~%.3f (w=%d h=%d)", axis, imgW, imgH, got, want, w, h)
		}
		// The chosen dimension is locked to the full terminal; the other may
		// overflow (scroll/wrap) and is intentionally not clamped.
		if axis == "W" {
			if w != termW {
				t.Errorf("text fit W %dx%d: width=%d, want full termW %d", imgW, imgH, w, termW)
			}
		} else {
			if h != termH-statusLinesTerminal {
				t.Errorf("text fit H %dx%d: height=%d, want full available %d", imgW, imgH, h, termH-statusLinesTerminal)
			}
		}
	}
	checkText("H", 200, 200) // square: fills height
	checkText("W", 200, 200) // square: fills width
	checkText("H", 300, 100) // wide: fills height, width overflows (ok)
	checkText("W", 100, 300) // tall: fills width, height overflows (ok)

	// TGP/sixel are pixel dimensions: aspect must equal imgW/imgH, and the
	// chosen axis must fill the terminal.
	wt, ht := fitToTerminal(300, 100, termW, termH, "tgp", "H")
	if got, want := float64(wt)/float64(ht), 3.0; math.Abs(got-want) > 0.05 {
		t.Errorf("tgp fit H: aspect %.3f, want ~3.0 (w=%d h=%d)", got, wt, ht)
	}
	ww, _ := fitToTerminal(100, 300, termW, termH, "tgp", "W")
	cellW, _ := getCellSize()
	if ww != termW*cellW {
		t.Errorf("tgp fit W: width=%d, want full termPixelW %d", ww, termW*cellW)
	}

	// Bare/default axis ("") behaves like "H".
	_, h := fitToTerminal(200, 200, termW, termH, "rgb", "")
	if h != termH-statusLinesTerminal {
		t.Errorf("default axis: height=%d, want full available %d", h, termH-statusLinesTerminal)
	}
}

// TestParseFit verifies the --fit flag only selects an axis (H/W); scaling is
// handled by the separate --scale flag, so numeric/unknown values fall back to H.
func TestParseFit(t *testing.T) {
	cases := []struct {
		in       string
		wantAxis string
	}{
		{"", "H"},
		{"H", "H"},
		{"h", "H"},
		{"W", "W"},
		{"w", "W"},
		{"3", "H"},       // numbers are no longer a scale; default to H
		{"0.5", "H"},     // same
		{"garbage", "H"}, // unrecognized -> safe default
	}
	for _, c := range cases {
		if axis := parseFit(c.in); axis != c.wantAxis {
			t.Errorf("parseFit(%q) = %q, want %q", c.in, axis, c.wantAxis)
		}
	}
}

// TestValidateFlagsExitCode verifies that invalid flag values are usage errors
// (exit code 2) rather than generic runtime failures.
func TestValidateFlagsExitCode(t *testing.T) {
	oldMode, oldRotate, oldWidth, oldHeight, oldScale := mode, rotate, width, height, scale
	defer func() { mode, rotate, width, height, scale = oldMode, oldRotate, oldWidth, oldHeight, oldScale }()

	reset := func() {
		mode, rotate, width, height, scale = "rgb", 0, 0, 0, 1.0
	}

	cases := []struct {
		name   string
		mutate func()
	}{
		{"mode", func() { mode = "bogus" }},
		{"rotate", func() { rotate = 45 }},
		{"width", func() { width = -1 }},
		{"scale", func() { scale = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reset()
			tc.mutate()
			err := validateFlags()
			if err == nil {
				t.Fatal("validateFlags() returned nil for invalid value")
			}
			var ee *exitError
			if !errors.As(err, &ee) || ee.code != exitUsage {
				t.Errorf("expected usage exit code %d, got %T/%v", exitUsage, err, err)
			}
		})
	}
}

// TestLoadImageReturnsError verifies that filesystem failures are returned to
// Cobra (and printed on stderr by main) instead of terminating the process.
func TestLoadImageReturnsError(t *testing.T) {
	oldInput, oldPaste := input, paste
	defer func() { input, paste = oldInput, oldPaste }()
	input = "/nonexistent/pixu-test-image.png"
	paste = false

	if _, err := loadImage(nil); err == nil {
		t.Fatal("loadImage() should return an error for a missing file")
	}
}

// referenceFloydSteinberg is a slow, straightforward implementation used only
// as an oracle: it goes through image.At and calls quantize directly, so it
// stays independent of the production fast path (direct pixel access and the
// precomputed quantization table).
func referenceFloydSteinberg(src image.Image) image.Image {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	dst := image.NewNRGBA(b)
	const levels = 6
	span := w + 2
	cur := make([]float64, 3*span)
	next := make([]float64, 3*span)

	clamp := func(v float64) float64 {
		if v > 255 {
			return 255
		}
		if v < 0 {
			return 0
		}
		return v
	}

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r32, g32, b32, _ := src.At(b.Min.X+x, b.Min.Y+y).RGBA()
			r := clamp(float64(r32>>8) + cur[x])
			g := clamp(float64(g32>>8) + cur[span+x])
			bl := clamp(float64(b32>>8) + cur[2*span+x])

			nr, ng, nb := quantize(r, levels), quantize(g, levels), quantize(bl, levels)
			dst.SetNRGBA(b.Min.X+x, b.Min.Y+y, color.NRGBA{
				R: uint8(nr), G: uint8(ng), B: uint8(nb), A: 255,
			})

			dr, dg, db := r-nr, g-ng, bl-nb
			cur[x+1] += dr * 7 / 16
			cur[span+x+1] += dg * 7 / 16
			cur[2*span+x+1] += db * 7 / 16
			if x > 0 {
				next[x-1] += dr * 3 / 16
				next[span+x-1] += dg * 3 / 16
				next[2*span+x-1] += db * 3 / 16
			}
			next[x] += dr * 5 / 16
			next[span+x] += dg * 5 / 16
			next[2*span+x] += db * 5 / 16
			if x+1 < w {
				next[x+1] += dr / 16
				next[span+x+1] += dg / 16
				next[2*span+x+1] += db / 16
			}
		}
		cur, next = next, cur
		for i := range next {
			next[i] = 0
		}
	}
	return dst
}

// TestFloydSteinbergMatchesReference guards the LUT fast path against any
// behavioural drift: output must be byte-for-byte identical to the float path.
func TestFloydSteinbergMatchesReference(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 37, 23))
	for y := 0; y < 23; y++ {
		for x := 0; x < 37; x++ {
			src.Set(x, y, color.RGBA{
				R: uint8(x*11 + y*3),
				G: uint8(y*17 + x*5),
				B: uint8(x*y + 7),
				A: 255,
			})
		}
	}

	got := applyFloydSteinberg(src)
	want := referenceFloydSteinberg(src)
	b := src.Bounds()
	if got.Bounds() != b {
		t.Fatalf("bounds = %v, want %v", got.Bounds(), b)
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if g, w := got.At(x, y), want.At(x, y); g != w {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, g, w)
			}
		}
	}
}
