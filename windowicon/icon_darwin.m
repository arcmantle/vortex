#import <Cocoa/Cocoa.h>

void setWindowIconFromData(const void *data, int len) {
	NSData *imgData = [NSData dataWithBytes:data length:len];
	NSImage *img = [[NSImage alloc] initWithData:imgData];
	if (img) {
		[NSApp setApplicationIconImage:img];
	}
}
