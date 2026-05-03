#include "host_windows.h"
#ifdef _WIN32

static LRESULT CALLBACK vortexOverlayWndProc(HWND hwnd, UINT msg, WPARAM wp,
                                             LPARAM lp) {
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

static LRESULT CALLBACK vortexHostWndProc(HWND hwnd, UINT msg, WPARAM wp,
                                          LPARAM lp) {
  (void)wp;
  (void)lp;
  switch (msg) {
  case WM_SIZE: {
    RECT bounds;
    if (GetClientRect(hwnd, &bounds)) {
      for (HWND child = GetWindow(hwnd, GW_CHILD); child != NULL;
           child = GetWindow(child, GW_HWNDNEXT)) {
        SetWindowPos(child, NULL, 0, 0, bounds.right - bounds.left,
                     bounds.bottom - bounds.top, SWP_NOZORDER | SWP_NOACTIVATE);
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

HWND createHiddenHostWindow(int width, int height) {
  if (!ensureVortexHostWindowClass()) {
    return NULL;
  }

  RECT bounds = {0, 0, width, height};
  AdjustWindowRect(&bounds, WS_OVERLAPPEDWINDOW, FALSE);

  return CreateWindowExW(0, L"VortexWebviewHost", L"", WS_OVERLAPPEDWINDOW,
                         CW_USEDEFAULT, CW_USEDEFAULT,
                         bounds.right - bounds.left, bounds.bottom - bounds.top,
                         NULL, NULL, GetModuleHandleW(NULL), NULL);
}

HWND createOverlayWindow(HWND parent) {
  if (parent == NULL || !IsWindow(parent) ||
      !ensureVortexOverlayWindowClass()) {
    return NULL;
  }

  return CreateWindowExW(0, L"VortexWebviewOverlay", L"", WS_CHILD | WS_VISIBLE,
                         0, 0, 0, 0, parent, NULL, GetModuleHandleW(NULL),
                         NULL);
}

void destroyHostWindow(HWND hwnd) {
  if (hwnd != NULL && IsWindow(hwnd)) {
    DestroyWindow(hwnd);
  }
}

void layoutHostWindow(HWND hwnd) {
  if (hwnd == NULL || !IsWindow(hwnd)) {
    return;
  }
  SendMessageW(hwnd, WM_SIZE, 0, 0);
}

void showHostWindow(HWND hwnd) {
  if (hwnd == NULL || !IsWindow(hwnd)) {
    return;
  }
  ShowWindow(hwnd, SW_SHOW);
  UpdateWindow(hwnd);
}

void hideOverlayWindow(HWND hwnd) {
  if (hwnd == NULL || !IsWindow(hwnd)) {
    return;
  }
  ShowWindow(hwnd, SW_HIDE);
  DestroyWindow(hwnd);
}

HRESULT initHostCOM(void) {
  return CoInitializeEx(NULL, COINIT_APARTMENTTHREADED);
}

void uninitHostCOM(void) { CoUninitialize(); }

#endif
