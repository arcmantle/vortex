//go:build linux

package webview

/*
#cgo pkg-config: gtk+-3.0

#include <gtk/gtk.h>

static void setWindowIconFromData(void *window, const void *data, int len) {
	GdkPixbufLoader *loader = gdk_pixbuf_loader_new();
	gdk_pixbuf_loader_write(loader, (const guchar *)data, len, NULL);
	gdk_pixbuf_loader_close(loader, NULL);
	GdkPixbuf *pixbuf = gdk_pixbuf_loader_get_pixbuf(loader);
	if (pixbuf) {
		gtk_window_set_icon(GTK_WINDOW(window), pixbuf);
	}
	g_object_unref(loader);
}
*/
import "C"

import (
	"unsafe"

	webview "github.com/webview/webview_go"
)

func open(title, url string, width, height int) {
	w := webview.New(false)
	if w == nil {
		return
	}
	defer w.Destroy()
	w.SetTitle(title)
	w.SetSize(width, height, webview.HintNone)
	C.setWindowIconFromData(w.Window(), unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
	w.Navigate(url)
	w.Run()
}
