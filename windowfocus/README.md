# windowfocus

`windowfocus` is a small Go package for bringing an existing native window to the foreground.

It only does one thing:

```go
windowfocus.Focus(handle)
```

## Scope

- Focus an existing native window
- Best-effort behavior on supported platforms
- No window creation
- No icon handling
- No toolkit bindings
- No UI-thread dispatch

## Supported Platforms

- macOS with `cgo`
- Linux with `cgo`
- Windows with `cgo`

For unsupported platforms or `!cgo` builds, `Focus` is a no-op.

## API

```go
package windowfocus

func Focus(window unsafe.Pointer)
```

`window` must be a native window handle supplied by your UI toolkit.

Examples:

- macOS: `NSWindow *`
- Linux GTK: `GtkWindow *`
- Windows: `HWND`

## Important

The caller is responsible for invoking `Focus` on the correct UI thread for the toolkit in use.

This package does not know how to marshal onto a toolkit event loop.

## Example

```go
w.Dispatch(func() {
	windowfocus.Focus(w.Window())
})
```

## Module

```go
module arcmantle/windowfocus
```