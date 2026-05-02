#pragma once
#ifdef _WIN32
#include <windows.h>

#ifdef __cplusplus
extern "C" {
#endif

void setWindowIconFromData(void *hwnd, const void *data, int len);

#ifdef __cplusplus
}
#endif
#endif /* _WIN32 */
