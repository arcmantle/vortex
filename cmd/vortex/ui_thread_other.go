//go:build !darwin

package main

import (
	"runtime"
	"sync"
)

type uiThreadRunner struct {
	work chan func()
	once sync.Once
}

func newUIThreadRunner() *uiThreadRunner {
	runner := &uiThreadRunner{work: make(chan func(), 1)}
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		for fn := range runner.work {
			if fn != nil {
				fn()
			}
		}
	}()
	return runner
}

func (runner *uiThreadRunner) Post(fn func()) {
	if runner == nil || fn == nil {
		return
	}
	runner.work <- fn
}

// Close shuts down the UI thread goroutine. Safe to call multiple times.
func (runner *uiThreadRunner) Close() {
	if runner == nil {
		return
	}
	runner.once.Do(func() { close(runner.work) })
}
