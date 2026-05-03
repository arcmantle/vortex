//go:build !cgo

package windowlifecycle

func configure(cfg Config) <-chan Event {
	ch := make(chan Event)
	close(ch)
	return ch
}

func installWindowDelegate(hideOnClose bool) {}

func showWindow() {}
