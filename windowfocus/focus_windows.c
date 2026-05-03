#include <windows.h>

void focusWindow(void *hwnd) {
	HWND window = (HWND)hwnd;
	if (!window) {
		return;
	}
	if (IsIconic(window)) {
		ShowWindow(window, SW_RESTORE);
	} else {
		ShowWindow(window, SW_SHOW);
	}
	BringWindowToTop(window);
	SetActiveWindow(window);
	SetForegroundWindow(window);
}
