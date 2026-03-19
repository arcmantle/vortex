//go:build darwin

package webview

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>

static void setAppIcon(const void *data, int len) {
	NSData *imgData = [NSData dataWithBytes:data length:len];
	NSImage *img = [[NSImage alloc] initWithData:imgData];
	if (img) {
		[NSApp setApplicationIconImage:img];
	}
}
*/
import "C"

import (
	"context"
	"unsafe"

	webview "github.com/webview/webview_go"
)

func init() {
	openWithContextImpl = openNativeWithContext
}

func openNativeWithContext(ctx context.Context, title, url string, width, height int) {
	w := webview.New(false)
	if w == nil {
		return
	}
	defer w.Destroy()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			w.Terminate()
		}()
	}
	w.SetTitle(title)
	w.SetSize(width, height, webview.HintNone)
	C.setAppIcon(unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
	w.Navigate(url)
	w.Run()
}
