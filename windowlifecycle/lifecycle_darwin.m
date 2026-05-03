#import <Cocoa/Cocoa.h>

// Forward declarations for Go callbacks (defined in lifecycle_darwin.go).
extern void goAppkitWindowHidden(void);
extern void goAppkitReopenRequest(void);
extern void goAppkitQuitRequest(void);

// ─── Application Delegate ───────────────────────────────────────────────────

@interface VortexAppDelegate : NSObject <NSApplicationDelegate>
@end

@implementation VortexAppDelegate

- (BOOL)applicationShouldTerminateAfterLastWindowClosed:(NSApplication *)app {
    (void)app;
    return NO;
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)app hasVisibleWindows:(BOOL)hasVisible {
    (void)app;
    if (!hasVisible) {
        goAppkitReopenRequest();
    }
    return YES;
}

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)app {
    (void)app;
    goAppkitQuitRequest();
    // Return Cancel — let Go decide whether to actually quit.
    return NSTerminateCancel;
}

@end

// ─── Window Delegate ────────────────────────────────────────────────────────

static BOOL g_hideOnClose = NO;

@interface VortexWindowDelegate : NSObject <NSWindowDelegate>
@property (nonatomic, assign) id originalDelegate;
@end

@implementation VortexWindowDelegate

- (BOOL)windowShouldClose:(NSWindow *)window {
    if (g_hideOnClose) {
        [window orderOut:nil];
        goAppkitWindowHidden();
        return NO;  // Prevent destruction.
    }
    return YES;
}

// Forward windowWillClose: to the original delegate (webview's) if we allow close.
- (void)windowWillClose:(NSNotification *)notification {
    if (self.originalDelegate && [self.originalDelegate respondsToSelector:@selector(windowWillClose:)]) {
        [self.originalDelegate windowWillClose:notification];
    }
}

// Forward any other delegate methods webview might need.
- (id)forwardingTargetForSelector:(SEL)aSelector {
    if (self.originalDelegate && [self.originalDelegate respondsToSelector:aSelector]) {
        return self.originalDelegate;
    }
    return [super forwardingTargetForSelector:aSelector];
}

- (BOOL)respondsToSelector:(SEL)aSelector {
    if ([super respondsToSelector:aSelector]) {
        return YES;
    }
    if (self.originalDelegate && [self.originalDelegate respondsToSelector:aSelector]) {
        return YES;
    }
    return NO;
}

@end

// ─── C API ──────────────────────────────────────────────────────────────────

static VortexAppDelegate *g_appDelegate = nil;
static VortexWindowDelegate *g_windowDelegate = nil;

void vortexInstallAppDelegate(void) {
    @autoreleasepool {
        NSApplication *app = [NSApplication sharedApplication];
        g_appDelegate = [[VortexAppDelegate alloc] init];
        [app setDelegate:g_appDelegate];
    }
}

void vortexInstallWindowDelegate(int hideOnClose) {
    @autoreleasepool {
        g_hideOnClose = hideOnClose ? YES : NO;
        NSWindow *window = [[NSApplication sharedApplication] mainWindow];
        if (!window) {
            window = [[NSApplication sharedApplication] keyWindow];
        }
        if (!window) {
            NSArray *windows = [[NSApplication sharedApplication] windows];
            for (NSWindow *w in windows) {
                if ([w canBecomeMainWindow]) {
                    window = w;
                    break;
                }
            }
        }
        if (!window) {
            return;
        }
        g_windowDelegate = [[VortexWindowDelegate alloc] init];
        g_windowDelegate.originalDelegate = [window delegate];
        [window setDelegate:g_windowDelegate];
    }
}

void vortexShowMainWindow(void) {
    @autoreleasepool {
        NSWindow *window = [[NSApplication sharedApplication] mainWindow];
        if (!window) {
            window = [[NSApplication sharedApplication] keyWindow];
        }
        if (!window) {
            NSArray *windows = [[NSApplication sharedApplication] windows];
            for (NSWindow *w in windows) {
                if ([w canBecomeMainWindow]) {
                    window = w;
                    break;
                }
            }
        }
        if (window) {
            [window makeKeyAndOrderFront:nil];
            [[NSApplication sharedApplication] activateIgnoringOtherApps:YES];
        }
    }
}
