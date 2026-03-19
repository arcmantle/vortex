package webview

import (
	"arcmantle/windowfocus"
	"arcmantle/windowicon"

	webviewlib "github.com/webview/webview_go"
)

type nativeController struct {
	w webviewlib.WebView
}

func (c nativeController) setIcon(icon []byte) {
	windowicon.Set(c.w.Window(), icon)
}

func (c nativeController) Focus() {
	c.w.Dispatch(func() {
		windowfocus.Focus(c.w.Window())
	})
}
