//go:build cgo && (darwin || linux)

package webview

import (
	"context"
	"log"

	"arcmantle/windowfocus"

	webviewlib "github.com/webview/webview_go"
)

func openWithContext(ctx context.Context, title, url string, width, height int, options windowOptions, onReady func(Controller)) {
	windowfocus.ShowApp()
	w := webviewlib.New(false)
	if w == nil {
		log.Printf("webview: failed to initialize native window")
		return
	}
	defer w.Destroy()
	runDone := make(chan struct{})
	if ctx != nil {
		go func() {
			select {
			case <-ctx.Done():
				w.Terminate()
			case <-runDone:
			}
		}()
	}
	controller := nativeController{w: w}
	w.SetTitle(title)
	w.SetHtml(loadingDocument())
	var sizeHint webviewlib.Hint = webviewlib.HintNone
	if options.dialog {
		sizeHint = webviewlib.HintFixed
	}
	w.SetSize(width, height, sizeHint)
	controller.setIcon(iconPNG)
	if err := w.Bind("vortexOpenExternal", func(target string) error {
		return OpenExternalURL(target)
	}); err != nil {
		log.Printf("webview external browser bridge bind failed: %v", err)
	}
	controller.Focus()
	if onReady != nil {
		onReady(controller)
	}
	w.Dispatch(func() {
		w.Navigate(url)
	})
	w.Run()
	close(runDone)
}

func loadingDocument() string {
	return `<!doctype html>
<html lang="en">
	<head>
		<meta charset="utf-8">
		<meta name="viewport" content="width=device-width, initial-scale=1">
		<title>Vortex</title>
		<style>
			:root {
				color-scheme: dark;
				background: #1e1e1e;
				color: #d4d4d4;
			}
			* {
				box-sizing: border-box;
			}
			html, body {
				width: 100%;
				height: 100%;
				margin: 0;
				overflow: hidden;
				background: #1e1e1e;
				font-family: "Segoe UI", system-ui, sans-serif;
			}
			body {
				display: grid;
				place-items: center;
			}
			.shell {
				display: grid;
				gap: 12px;
				justify-items: center;
			}
			.mark {
				width: 56px;
				height: 56px;
				border-radius: 16px;
				background: linear-gradient(160deg, #2d2d30, #252526);
				box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.06);
			}
			.label {
				font-size: 14px;
				letter-spacing: 0.08em;
				text-transform: uppercase;
				color: #9da5b4;
			}
		</style>
	</head>
	<body>
		<div class="shell" aria-label="Loading Vortex">
			<div class="mark"></div>
			<div class="label">Opening Vortex</div>
		</div>
	</body>
</html>`
}
