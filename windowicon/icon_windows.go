//go:build windows && cgo

package windowicon

/*
#cgo LDFLAGS: -lgdiplus -lole32

#include <string.h>
#include <windows.h>
#include <objidl.h>
#include <gdiplus.h>

static void setWindowIconFromData(void *hwnd, const void *data, int len) {
	HWND window = (HWND)hwnd;
	if (!window || !data || len <= 0) {
		return;
	}

	GdiplusStartupInput startupInput;
	ULONG_PTR token = 0;
	ZeroMemory(&startupInput, sizeof(startupInput));
	startupInput.GdiplusVersion = 1;
	if (GdiplusStartup(&token, &startupInput, NULL) != Ok) {
		return;
	}

	HGLOBAL hGlobal = GlobalAlloc(GMEM_MOVEABLE, (SIZE_T)len);
	if (!hGlobal) {
		GdiplusShutdown(token);
		return;
	}

	void *buffer = GlobalLock(hGlobal);
	if (!buffer) {
		GlobalFree(hGlobal);
		GdiplusShutdown(token);
		return;
	}
	memcpy(buffer, data, (size_t)len);
	GlobalUnlock(hGlobal);

	IStream *stream = NULL;
	if (CreateStreamOnHGlobal(hGlobal, TRUE, &stream) != S_OK) {
		GlobalFree(hGlobal);
		GdiplusShutdown(token);
		return;
	}

	GpBitmap *bitmap = NULL;
	if (GdipCreateBitmapFromStream(stream, &bitmap) == Ok && bitmap) {
		HICON iconHandle = NULL;
		if (GdipCreateHICONFromBitmap(bitmap, &iconHandle) == Ok && iconHandle) {
			SendMessageW(window, WM_SETICON, ICON_BIG, (LPARAM)iconHandle);
			SendMessageW(window, WM_SETICON, ICON_SMALL, (LPARAM)iconHandle);
		}
		GdipDisposeImage((GpImage *)bitmap);
	}

	stream->lpVtbl->Release(stream);
	GdiplusShutdown(token);
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
