//go:build linux && cgo

package windowfocus

/*
#cgo pkg-config: gtk+-3.0

#include <gtk/gtk.h>

static void focusWindow(void *window) {
	if (!window) {
		return;
	}
	gtk_window_deiconify(GTK_WINDOW(window));
	gtk_window_present(GTK_WINDOW(window));
}
*/
import "C"

import "unsafe"

func showApp() {}

func hideApp() {}

func focus(window unsafe.Pointer) {
	if window == nil {
		return
	}

	C.focusWindow(window)
}
