#include <gtk/gtk.h>

void focusWindow(void *window) {
	if (!window) {
		return;
	}
	gtk_window_deiconify(GTK_WINDOW(window));
	gtk_window_present(GTK_WINDOW(window));
}
