// Copyright © 2026, Vitalii Tereshchuk | DOTOCA.NET
// Read image from macOS clipboard via NSPasteboard with TIFF fallback

//go:build darwin && !ios

#import <Foundation/Foundation.h>
#import <Cocoa/Cocoa.h>

// readClipboardImage reads image data from the system clipboard.
// It tries PNG first, then TIFF, and converts to PNG bytes.
// Returns allocated buffer and size, caller must free with free().
// Returns NULL with *outLen=0 if no image is found.
void readClipboardImage(void **out, unsigned int *outLen) {
    *out = NULL;
    *outLen = 0;

    NSPasteboard *pb = [NSPasteboard generalPasteboard];

    // 1. Try PNG directly
    NSData *data = [pb dataForType:NSPasteboardTypePNG];
    if (data != nil && [data length] > 0) {
        NSUInteger siz = [data length];
        *out = malloc(siz);
        [data getBytes:*out length:siz];
        *outLen = (unsigned int)siz;
        return;
    }

    // 2. Try TIFF, convert to PNG via NSImage
    NSImage *img = [[NSImage alloc] initWithPasteboard:pb];
    if (img == nil) {
        // fallback: try reading TIFF data directly
        NSData *tiffData = [pb dataForType:NSPasteboardTypeTIFF];
        if (tiffData != nil && [tiffData length] > 0) {
            img = [[NSImage alloc] initWithData:tiffData];
        }
    }
    if (img == nil) {
        return;
    }

    NSData *tiffRep = [img TIFFRepresentation];
    if (tiffRep == nil) {
        return;
    }

    NSBitmapImageRep *rep = [[NSBitmapImageRep alloc] initWithData:tiffRep];
    if (rep == nil) {
        return;
    }

    NSData *pngRep = [rep representationUsingType:NSBitmapImageFileTypePNG properties:@{}];
    if (pngRep != nil && [pngRep length] > 0) {
        NSUInteger siz = [pngRep length];
        *out = malloc(siz);
        [pngRep getBytes:*out length:siz];
        *outLen = (unsigned int)siz;
    }
}

// readClipboardText reads text from the system clipboard.
// Returns allocated buffer and size, caller must free with free().
// Returns NULL with *outLen=0 if no text is found.
void readClipboardText(void **out, unsigned int *outLen) {
    *out = NULL;
    *outLen = 0;

    NSPasteboard *pb = [NSPasteboard generalPasteboard];
    NSData *data = [pb dataForType:NSPasteboardTypeString];
    if (data == nil || [data length] == 0) {
        return;
    }
    NSUInteger siz = [data length];
    *out = malloc(siz);
    [data getBytes:*out length:siz];
    *outLen = (unsigned int)siz;
}
