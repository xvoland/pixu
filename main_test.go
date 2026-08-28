package main

import (
	"image"
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

// TestEnvPrecedence verifies the documented priority: CLI flag > env > default.
// It builds a command with the same flags (bound to the same globals) that
// applyEnvDefaults consults, avoiding a dependency on the package-private rootCmd.
func TestEnvPrecedence(t *testing.T) {
	reset := func() {
		width = 0
		scale = 1.0
		mode = "rgb"
	}
	newCmd := func() *cobra.Command {
		cmd := &cobra.Command{}
		cmd.Flags().IntVarP(&width, "width", "w", 0, "")
		cmd.Flags().Float64VarP(&scale, "scale", "S", 1.0, "")
		cmd.Flags().StringVarP(&mode, "mode", "m", "rgb", "")
		return cmd
	}
	cases := []struct {
		name, envKey, envVal, args string
		get  func() interface{}
		want interface{}
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

			cmd := newCmd()
			cmd.ParseFlags(strings.Fields(c.args))
			applyEnvDefaults(cmd)

			if got := c.get(); got != c.want {
				t.Errorf("got %v, want %v (env=%s=%q args=%q)", got, c.want, c.envKey, c.envVal, c.args)
			}
		})
	}
}
