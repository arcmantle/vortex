#pragma once
#ifdef _WIN32
#include <objbase.h>
#include <windows.h>

HWND createHiddenHostWindow(int width, int height);
HWND createOverlayWindow(HWND parent);
void destroyHostWindow(HWND hwnd);
void layoutHostWindow(HWND hwnd);
void showHostWindow(HWND hwnd);
void hideOverlayWindow(HWND hwnd);
HRESULT initHostCOM(void);
void uninitHostCOM(void);

#endif
