//go:build cgo && (darwin || linux || windows)

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

func (c nativeController) Close() {
	c.w.Dispatch(func() {
		c.w.Terminate()
	})
}

func (c nativeController) Focus() {
	c.w.Dispatch(func() {
		windowfocus.Focus(c.w.Window())
	})
}

func (c nativeController) Hide() {
	c.w.Dispatch(func() {
		windowfocus.HideWindow(c.w.Window())
	})
}

func (c nativeController) Show() {
	c.w.Dispatch(func() {
		windowfocus.ShowWindow(c.w.Window())
	})
}
