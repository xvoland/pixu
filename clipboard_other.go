// Copyright © 2026, Vitalii Tereshchuk | DOTOCA.NET

//go:build !darwin

package main

import (
	"golang.design/x/clipboard"
)

func clipboardReadImage() []byte {
	if err := clipboard.Init(); err != nil {
		return nil
	}
	return clipboard.Read(clipboard.FmtImage)
}

func clipboardReadText() string {
	if err := clipboard.Init(); err != nil {
		return ""
	}
	return string(clipboard.Read(clipboard.FmtText))
}
