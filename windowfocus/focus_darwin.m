#import <Cocoa/Cocoa.h>

void showApplication(void) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app setActivationPolicy:NSApplicationActivationPolicyRegular];
		[app unhide:nil];
	}
}

void hideApplication(void) {
	@autoreleasepool {
		NSApplication *app = [NSApplication sharedApplication];
		[app hide:nil];
		[app setActivationPolicy:NSApplicationActivationPolicyAccessory];
	}
}

void focusWindow(void *windowPtr) {
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

void hideNativeWindow(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (__bridge NSWindow *)windowPtr;
		if (!window) {
			return;
		}
		[window orderOut:nil];
	}
}

void showNativeWindow(void *windowPtr) {
	@autoreleasepool {
		NSWindow *window = (__bridge NSWindow *)windowPtr;
		if (!window) {
			return;
		}
		[window makeKeyAndOrderFront:nil];
		[[NSApplication sharedApplication] activateIgnoringOtherApps:YES];
	}
}
