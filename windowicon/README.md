# windowicon

`windowicon` is a small Go package for applying icon data to an existing native window.

It only does one thing:

```go
windowicon.Set(handle, icon)
```

## Scope

- Apply icon data to an existing native window
- Best-effort behavior on supported platforms
- No window creation
- No toolkit bindings
- No UI-thread dispatch

## Supported Platforms

- macOS with `cgo`
- Linux with `cgo`
- Windows with `cgo`

For unsupported platforms or `!cgo` builds, `Set` is a no-op.

## API

```go
package windowicon

func Set(window unsafe.Pointer, icon []byte)
```

`window` must be a native window handle supplied by your UI toolkit.

## Important

The caller is responsible for invoking `Set` on the correct UI thread for the toolkit in use.

This package does not know how to marshal onto a toolkit event loop.