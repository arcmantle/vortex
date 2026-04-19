//go:build windows && cgo

package webview

/*
#cgo LDFLAGS: -lgdi32 -lole32

#include <windows.h>
#include <objbase.h>

static LRESULT CALLBACK vortexOverlayWndProc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
	(void)wp;
	(void)lp;
	switch (msg) {
	case WM_ERASEBKGND:
		return 1;
	case WM_PAINT: {
		PAINTSTRUCT ps;
		RECT bounds;
		HDC dc = BeginPaint(hwnd, &ps);
		GetClientRect(hwnd, &bounds);
		HBRUSH brush = CreateSolidBrush(RGB(0x1e, 0x1e, 0x1e));
		FillRect(dc, &bounds, brush);
		DeleteObject(brush);
		EndPaint(hwnd, &ps);
		return 0;
	}
	default:
		return DefWindowProcW(hwnd, msg, wp, lp);
	}
}

static LRESULT CALLBACK vortexHostWndProc(HWND hwnd, UINT msg, WPARAM wp, LPARAM lp) {
	(void)wp;
	(void)lp;
	switch (msg) {
	case WM_SIZE: {
		RECT bounds;
		if (GetClientRect(hwnd, &bounds)) {
			for (HWND child = GetWindow(hwnd, GW_CHILD); child != NULL; child = GetWindow(child, GW_HWNDNEXT)) {
				SetWindowPos(child, NULL, 0, 0,
					bounds.right - bounds.left,
					bounds.bottom - bounds.top,
					SWP_NOZORDER | SWP_NOACTIVATE);
			}
		}
		return 0;
	}
	case WM_CLOSE:
		DestroyWindow(hwnd);
		return 0;
	case WM_DESTROY:
		PostQuitMessage(0);
		return 0;
	default:
		return DefWindowProcW(hwnd, msg, wp, lp);
	}
}

static ATOM ensureVortexHostWindowClass(void) {
	static ATOM atom = 0;
	if (atom != 0) {
		return atom;
	}

	WNDCLASSEXW wc;
	ZeroMemory(&wc, sizeof(wc));
	wc.cbSize = sizeof(wc);
	wc.hInstance = GetModuleHandleW(NULL);
	wc.lpfnWndProc = vortexHostWndProc;
	wc.lpszClassName = L"VortexWebviewHost";
	wc.hCursor = LoadCursorW(NULL, (LPCWSTR)IDC_ARROW);
	wc.hbrBackground = CreateSolidBrush(RGB(0x1e, 0x1e, 0x1e));
	atom = RegisterClassExW(&wc);
	return atom;
}

static ATOM ensureVortexOverlayWindowClass(void) {
	static ATOM atom = 0;
	if (atom != 0) {
		return atom;
	}

	WNDCLASSEXW wc;
	ZeroMemory(&wc, sizeof(wc));
	wc.cbSize = sizeof(wc);
	wc.hInstance = GetModuleHandleW(NULL);
	wc.lpfnWndProc = vortexOverlayWndProc;
	wc.lpszClassName = L"VortexWebviewOverlay";
	wc.hCursor = LoadCursorW(NULL, (LPCWSTR)IDC_ARROW);
	atom = RegisterClassExW(&wc);
	return atom;
}

static HWND createHiddenHostWindow(int width, int height) {
	if (!ensureVortexHostWindowClass()) {
		return NULL;
	}

	RECT bounds = {0, 0, width, height};
	AdjustWindowRect(&bounds, WS_OVERLAPPEDWINDOW, FALSE);

	return CreateWindowExW(
		0,
		L"VortexWebviewHost",
		L"",
		WS_OVERLAPPEDWINDOW,
		CW_USEDEFAULT,
		CW_USEDEFAULT,
		bounds.right - bounds.left,
		bounds.bottom - bounds.top,
		NULL,
		NULL,
		GetModuleHandleW(NULL),
		NULL
	);
}

static HWND createOverlayWindow(HWND parent) {
	if (parent == NULL || !IsWindow(parent) || !ensureVortexOverlayWindowClass()) {
		return NULL;
	}

	return CreateWindowExW(
		0,
		L"VortexWebviewOverlay",
		L"",
		WS_CHILD | WS_VISIBLE,
		0,
		0,
		0,
		0,
		parent,
		NULL,
		GetModuleHandleW(NULL),
		NULL
	);
}

static void destroyHostWindow(HWND hwnd) {
	if (hwnd != NULL && IsWindow(hwnd)) {
		DestroyWindow(hwnd);
	}
}

static void layoutHostWindow(HWND hwnd) {
	if (hwnd == NULL || !IsWindow(hwnd)) {
		return;
	}
	SendMessageW(hwnd, WM_SIZE, 0, 0);
}

static void showHostWindow(HWND hwnd) {
	if (hwnd == NULL || !IsWindow(hwnd)) {
		return;
	}
	ShowWindow(hwnd, SW_SHOW);
	UpdateWindow(hwnd);
}

static void hideOverlayWindow(HWND hwnd) {
	if (hwnd == NULL || !IsWindow(hwnd)) {
		return;
	}
	ShowWindow(hwnd, SW_HIDE);
	DestroyWindow(hwnd);
}

static HRESULT initHostCOM(void) {
	return CoInitializeEx(NULL, COINIT_APARTMENTTHREADED);
}

static void uninitHostCOM(void) {
	CoUninitialize();
}
*/
import "C"

import (
	"context"
	"fmt"
	"log"
	"unsafe"

	"arcmantle/windowfocus"

	webviewlib "github.com/webview/webview_go"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32DLL                              = windows.NewLazySystemDLL("user32.dll")
	dwmapiDLL                              = windows.NewLazySystemDLL("dwmapi.dll")
	procGetWindow                          = user32DLL.NewProc("GetWindow")
	procShowWindow                         = user32DLL.NewProc("ShowWindow")
	procIsWindow                           = user32DLL.NewProc("IsWindow")
	procGetWindowLongPtr                   = user32DLL.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtr                   = user32DLL.NewProc("SetWindowLongPtrW")
	procSetLayeredWindowAttributes         = user32DLL.NewProc("SetLayeredWindowAttributes")
	procDwmSetWindowAttribute              = dwmapiDLL.NewProc("DwmSetWindowAttribute")
	gwChild                        uintptr = 5
	gwHwndNext                     uintptr = 2
	swHide                         uintptr = 0
	swShow                         uintptr = 5
	gwlExStyle                     uintptr = ^uintptr(19)
	wsExLayered                    uintptr = 0x00080000
	lwaAlpha                       uintptr = 0x00000002
	dwmaUseImmersiveDarkMode               = uintptr(20)
	dwmaUseImmersiveDarkModeLegacy         = uintptr(19)
)

func prefersDarkWindowTheme() bool {
	key, err := registry.OpenKey(registry.CURRENT_USER, `Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer key.Close()

	if value, _, err := key.GetIntegerValue("AppsUseLightTheme"); err == nil {
		return value == 0
	}
	if value, _, err := key.GetIntegerValue("SystemUsesLightTheme"); err == nil {
		return value == 0
	}
	return false
}

func applyWindowTheme(hwnd unsafe.Pointer) {
	handle := uintptr(hwnd)
	if handle == 0 || !isWindow(handle) {
		return
	}

	useDark := uint32(0)
	if prefersDarkWindowTheme() {
		useDark = 1
	}

	procDwmSetWindowAttribute.Call(handle, dwmaUseImmersiveDarkMode, uintptr(unsafe.Pointer(&useDark)), unsafe.Sizeof(useDark))
	procDwmSetWindowAttribute.Call(handle, dwmaUseImmersiveDarkModeLegacy, uintptr(unsafe.Pointer(&useDark)), unsafe.Sizeof(useDark))
}

func setWindowAlpha(hwnd unsafe.Pointer, alpha byte) {
	handle := uintptr(hwnd)
	if handle == 0 || !isWindow(handle) {
		return
	}
	style, _, _ := procGetWindowLongPtr.Call(handle, gwlExStyle)
	if style&wsExLayered == 0 {
		procSetWindowLongPtr.Call(handle, gwlExStyle, style|wsExLayered)
	}
	procSetLayeredWindowAttributes.Call(handle, 0, uintptr(alpha), lwaAlpha)
}

func setHostChildVisibility(hostWindow, except unsafe.Pointer, visible bool) {
	host := uintptr(hostWindow)
	if host == 0 || !isWindow(host) {
		return
	}
	exceptHandle := uintptr(except)
	showFlag := swHide
	if visible {
		showFlag = swShow
	}
	child, _, _ := procGetWindow.Call(host, gwChild)
	for child != 0 {
		if child != exceptHandle {
			procShowWindow.Call(child, showFlag)
		}
		next, _, _ := procGetWindow.Call(child, gwHwndNext)
		child = next
	}
}

func isWindow(hwnd uintptr) bool {
	result, _, _ := procIsWindow.Call(hwnd)
	return result != 0
}

func openWithContext(ctx context.Context, title, url string, width, height int, onReady func(Controller)) {
	windowfocus.ShowApp()

	hostWindow, releaseHost, err := createHostWindow(width, height)
	if err != nil {
		log.Printf("webview host window setup failed: %v", err)
		return
	}
	defer releaseHost()
	applyWindowTheme(hostWindow)

	w := webviewlib.NewWindow(false, hostWindow)
	if w == nil {
		log.Printf("webview: failed to initialize native window")
		return
	}
	defer w.Destroy()

	runDone := make(chan struct{})
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				w.Terminate()
			case <-runDone:
			}
		}()
	}

	controller := nativeController{w: w}
	appReady := make(chan struct{}, 1)
	w.SetTitle(title)
	w.SetSize(width, height, webviewlib.HintNone)
	controller.setIcon(iconPNG)
	if err := w.Bind("vortexAppReady", func() {
		select {
		case appReady <- struct{}{}:
		default:
		}
	}); err != nil {
		log.Printf("webview app ready bridge bind failed: %v", err)
	}
	if err := w.Bind("vortexOpenExternal", func(target string) error {
		return OpenExternalURL(target)
	}); err != nil {
		log.Printf("webview external browser bridge bind failed: %v", err)
	}

	overlay := unsafe.Pointer(C.createOverlayWindow((C.HWND)(hostWindow)))
	C.layoutHostWindow((C.HWND)(hostWindow))
	w.Navigate(url)
	setHostChildVisibility(hostWindow, overlay, false)
	setWindowAlpha(hostWindow, 0)
	C.showHostWindow((C.HWND)(hostWindow))
	controller.Focus()
	if onReady != nil {
		onReady(controller)
	}
	go func() {
		w.Dispatch(func() {
			setWindowAlpha(hostWindow, 255)
		})
	}()

	go func() {
		if ctx == nil {
			<-appReady
			w.Dispatch(func() {
				setHostChildVisibility(hostWindow, overlay, true)
				C.hideOverlayWindow((C.HWND)(overlay))
			})
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-runDone:
			return
		case <-appReady:
			w.Dispatch(func() {
				setHostChildVisibility(hostWindow, overlay, true)
				C.hideOverlayWindow((C.HWND)(overlay))
			})
		}
	}()

	w.Run()
	close(runDone)
}

func createHostWindow(width, height int) (unsafe.Pointer, func(), error) {
	hr := C.initHostCOM()
	switch hr {
	case C.S_OK, C.S_FALSE:
	default:
		return nil, nil, fmt.Errorf("CoInitializeEx failed: 0x%x", uint32(hr))
	}

	hwnd := C.createHiddenHostWindow(C.int(width), C.int(height))
	if hwnd == nil {
		C.uninitHostCOM()
		return nil, nil, fmt.Errorf("CreateWindowExW failed")
	}

	released := false
	release := func() {
		if released {
			return
		}
		released = true
		C.destroyHostWindow((C.HWND)(hwnd))
		C.uninitHostCOM()
	}

	return unsafe.Pointer(hwnd), release, nil
}
