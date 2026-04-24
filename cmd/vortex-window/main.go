// vortex-window is a thin native webview host. It opens a single window
// pointing at a URL and exits when the window is closed. The main vortex
// binary spawns this process for the GUI; keeping it separate means the
// host process stays a console-subsystem app and avoids the ConPTY / PTY
// issues caused by -H=windowsgui on Windows.
//
// Communication with the parent process:
//
//	stdout: "READY\n" once the window is visible.
//	stdin:  line-delimited commands from the parent:
//	        "FOCUS\n"  — bring window to foreground
//	        "CLOSE\n"  — close window gracefully
//	        EOF        — close window (parent exited)
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"arcmantle/vortex/internal/webview"
)

func main() {
	title := flag.String("title", "Vortex", "window title")
	url := flag.String("url", "", "URL to navigate to (required)")
	width := flag.Int("width", 1280, "initial window width")
	height := flag.Int("height", 800, "initial window height")
	flag.Parse()

	if *url == "" {
		fmt.Fprintln(os.Stderr, "vortex-window: --url is required")
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var (
		mu         sync.Mutex
		controller webview.Controller
		readySent  bool
	)

	// Read commands from the parent on stdin.
	go func() {
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			cmd := strings.TrimSpace(scanner.Text())
			switch cmd {
			case "FOCUS":
				mu.Lock()
				c := controller
				mu.Unlock()
				if c != nil {
					c.Focus()
				}
			case "CLOSE":
				stop()
				return
			}
		}
		// stdin closed (parent exited) — shut down.
		stop()
	}()

	// Signal readiness to the parent process by writing to stdout.
	ready := func(c webview.Controller) {
		mu.Lock()
		controller = c
		readySent = true
		mu.Unlock()
		fmt.Fprintln(os.Stdout, "READY")
		if c != nil {
			c.Focus()
		}
	}

	log.SetPrefix("[vortex-window] ")
	log.Printf("starting native webview host for %s", *url)
	webview.OpenWithContextAndReady(ctx, *title, *url, *width, *height, ready)
	if !readySent {
		log.Printf("native webview host returned before READY")
		os.Exit(1)
	}
}
