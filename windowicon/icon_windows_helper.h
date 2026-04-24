#pragma once
#include <windows.h>

#ifdef __cplusplus
extern "C" {
#endif

void setWindowIconFromData(void *hwnd, const void *data, int len);

#ifdef __cplusplus
}
#endif
