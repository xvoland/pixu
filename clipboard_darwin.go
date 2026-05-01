// Copyright © 2026, Vitalii Tereshchuk | DOTOCA.NET

//go:build darwin && !ios

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <stdlib.h>

extern void readClipboardImage(char **out, unsigned int *outLen);
extern void readClipboardText(char **out, unsigned int *outLen);
*/
import "C"
import "unsafe"

func clipboardReadImage() []byte {
	var out *C.char
	var outLen C.uint
	C.readClipboardImage(&out, &outLen)
	if out == nil || outLen == 0 {
		return nil
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoBytes(unsafe.Pointer(out), C.int(outLen))
}

func clipboardReadText() string {
	var out *C.char
	var outLen C.uint
	C.readClipboardText(&out, &outLen)
	if out == nil || outLen == 0 {
		return ""
	}
	defer C.free(unsafe.Pointer(out))
	return C.GoStringN(out, C.int(outLen))
}
