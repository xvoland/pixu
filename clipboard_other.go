// Copyright © 2026, Vitalii Tereshchuk | DOTOCA.NET

//go:build !darwin

package main

import (
	"fmt"
	"log"
	"os"
	"runtime"

	"golang.design/x/clipboard"
)

// fatalClipboardInit reports a clipboard backend failure with a clear reason.
// On Linux the backend uses X11, so a missing X server under native Wayland is
// the usual cause; we surface that specifically instead of a generic error.
func fatalClipboardInit(err error) {
	msg := fmt.Sprintf("clipboard unavailable: %v", err)
	if runtime.GOOS == "linux" && os.Getenv("WAYLAND_DISPLAY") != "" {
		msg += " (native Wayland detected; the Linux clipboard backend requires X11/XWayland)"
	}
	log.Fatalf(msg)
}

func clipboardReadImage() []byte {
	if err := clipboard.Init(); err != nil {
		fatalClipboardInit(err)
	}
	return clipboard.Read(clipboard.FmtImage)
}

func clipboardReadText() string {
	if err := clipboard.Init(); err != nil {
		fatalClipboardInit(err)
	}
	return string(clipboard.Read(clipboard.FmtText))
}
