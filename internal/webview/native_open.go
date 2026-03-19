//go:build cgo && (darwin || linux || windows)

package webview

import (
	"context"

	webviewlib "github.com/webview/webview_go"
)

func openWithContext(ctx context.Context, title, url string, width, height int, onReady func(Controller)) {
	w := webviewlib.New(false)
	if w == nil {
		return
	}
	defer w.Destroy()
	if ctx != nil {
		go func() {
			<-ctx.Done()
			w.Terminate()
		}()
	}
	controller := nativeController{w: w}
	w.SetTitle(title)
	w.SetSize(width, height, webviewlib.HintNone)
	controller.setIcon(iconPNG)
	w.Navigate(url)
	controller.Focus()
	if onReady != nil {
		onReady(controller)
	}
	w.Run()
}
