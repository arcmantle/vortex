//go:build linux && cgo

package windowicon

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

import "unsafe"

func set(window unsafe.Pointer, icon []byte) {
	if window == nil {
		return
	}
	C.setWindowIconFromData(window, unsafe.Pointer(&icon[0]), C.int(len(icon)))
}
