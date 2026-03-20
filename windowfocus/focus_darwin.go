//go:build darwin && cgo

package windowfocus

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa

#include <Cocoa/Cocoa.h>

static void showApplication(void);
static void hideApplication(void);
static void focusWindow(void *windowPtr);

static void showApplication(void) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		[app unhide:nil];
	}
}

static void hideApplication(void) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app hide:nil];
		[app setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}

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

func showApp() {
	C.showApplication()
}

func hideApp() {
	C.hideApplication()
}

func focus(window unsafe.Pointer) {
	if window == nil {
		return
	}

	C.focusWindow(window)
}
