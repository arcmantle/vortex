//go:build windows

package webview

/*
#include <windows.h>

static void setWindowIcon(void *hwnd) {
	HMODULE hModule = GetModuleHandleW(NULL);
	HICON hIcon = (HICON)LoadImageW(hModule, L"APP", IMAGE_ICON, 0, 0,
		LR_DEFAULTSIZE | LR_SHARED);
	if (hIcon) {
		SendMessageW((HWND)hwnd, WM_SETICON, ICON_BIG, (LPARAM)hIcon);
		SendMessageW((HWND)hwnd, WM_SETICON, ICON_SMALL, (LPARAM)hIcon);
	}
}
*/
import "C"

import webview "github.com/webview/webview_go"

func open(title, url string, width, height int) {
	w := webview.New(false)
	if w == nil {
		return
	}
	defer w.Destroy()
	w.SetTitle(title)
	w.SetSize(width, height, webview.HintNone)
	C.setWindowIcon(w.Window())
	w.Navigate(url)
	w.Run()
}
