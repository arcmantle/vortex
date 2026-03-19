//go:build darwin && cgo

package windowfocus

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>

static void focusWindow(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (__bridge NSWindow *)windowPtr;
		if (!window) {
			return;
		}
		[[NSApplication sharedApplication] unhide:nil];
		if ([window isMiniaturized]) {
			[window deminiaturize:nil];
		}
		[window makeKeyAndOrderFront:nil];
		[window orderFrontRegardless];
		[[NSApplication sharedApplication] activateIgnoringOtherApps:YES];
	}
}
*/
import "C"

import "unsafe"

func focus(window unsafe.Pointer) {
	if window == nil {
		return
	}

	C.focusWindow(window)
}
