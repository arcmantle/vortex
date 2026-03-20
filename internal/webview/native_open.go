//go:build cgo && (darwin || linux || windows)

package webview

import (
	"context"
	"log"

	"arcmantle/windowfocus"

	webviewlib "github.com/webview/webview_go"
)

func openWithContext(ctx context.Context, title, url string, width, height int, onReady func(Controller)) {
	windowfocus.ShowApp()
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
	if err := w.Bind("vortexOpenExternal", func(target string) error {
		return OpenExternalURL(target)
	}); err != nil {
		log.Printf("webview external browser bridge bind failed: %v", err)
	}
	w.Navigate(url)
	controller.Focus()
	if onReady != nil {
		onReady(controller)
	}
	w.Run()
}
