// vortex (gui) is a thin native webview host. It opens a single window
// pointing at a URL and exits when the window is closed. The main vortex
// binary spawns this process for the GUI; keeping it separate means the
// host process stays a console-subsystem app and avoids the ConPTY / PTY
// issues caused by -H=windowsgui on Windows.
//
// Communication with the parent process:
//
//	stdout: "READY\n" once the window is visible.
//	        "HIDDEN\n" when window is hidden (darwin only).
//	stdin:  line-delimited commands from the parent:
//	        "FOCUS\n"  — bring window to foreground
//	        "SHOW\n"   — unhide window (darwin only)
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
	url := flag.String("url", "", "URL to navigate to (if empty, spawns host automatically)")
	width := flag.Int("width", 1280, "initial window width")
	height := flag.Int("height", 800, "initial window height")
	flag.Parse()

	log.SetPrefix("[vortex gui] ")

	// If no URL provided, enter self-launch mode: ensure the host is running
	// and discover its URL from the instance registry.
	targetURL := *url
	if targetURL == "" {
		resolved, err := selfLaunchURL()
		if err != nil {
			log.Printf("self-launch failed: %v", err)
			fmt.Fprintf(os.Stderr, "vortex: %v\n", err)
			os.Exit(1)
		}
		targetURL = resolved
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Self-launched mode: no parent process to communicate with.
	selfLaunched := *url == ""

	var (
		mu         sync.Mutex
		controller webview.Controller
		readySent  bool
		lifecycle  = newPlatformLifecycle()
	)

	// Platform-specific setup (installs AppKit delegates on darwin, no-op elsewhere).
	lifecycle.beforeWebview(stop)

	// Read commands from the parent on stdin (only when spawned by host).
	if !selfLaunched {
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
				case "SHOW":
					lifecycle.show()
				case "CLOSE":
					stop()
					return
				}
			}
			// stdin closed (parent exited) — shut down.
			stop()
		}()
	}

	// Signal readiness to the parent process by writing to stdout.
	ready := func(c webview.Controller) {
		mu.Lock()
		controller = c
		readySent = true
		mu.Unlock()

		// Platform-specific post-creation setup (window delegate on darwin).
		lifecycle.onReady(c)

		if !selfLaunched {
			fmt.Fprintln(os.Stdout, "READY")
		}
		if c != nil {
			c.Focus()
		}
	}

	log.Printf("starting native webview host for %s", targetURL)
	webview.OpenWithContextAndReady(ctx, *title, targetURL, *width, *height, ready)
	if !readySent {
		log.Printf("native webview host returned before READY")
		os.Exit(1)
	}
}
