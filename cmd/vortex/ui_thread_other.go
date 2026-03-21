//go:build !darwin

package main

import "runtime"

type uiThreadRunner struct {
	work chan func()
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
