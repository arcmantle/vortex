//go:build !darwin

package main

import (
	"runtime"
	"sync"
)

type uiThreadRunner struct {
	mu     sync.Mutex
	work   chan func()
	closed bool
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

// Post sends fn to the UI thread for execution. It is safe to call
// concurrently with Close; calls after Close are silently dropped.
func (runner *uiThreadRunner) Post(fn func()) {
	if runner == nil || fn == nil {
		return
	}
	runner.mu.Lock()
	if runner.closed {
		runner.mu.Unlock()
		return
	}
	select {
	case runner.work <- fn:
		runner.mu.Unlock()
		return
	default:
	}
	runner.mu.Unlock()
	// Channel full — send without lock so Close() can still proceed.
	// Close() may fire between the unlock and the send; recover the
	// resulting send-on-closed-channel panic.
	defer func() { recover() }()
	runner.work <- fn
}

// Close shuts down the UI thread goroutine. Safe to call multiple times.
func (runner *uiThreadRunner) Close() {
	if runner == nil {
		return
	}
	runner.mu.Lock()
	defer runner.mu.Unlock()
	if !runner.closed {
		runner.closed = true
		close(runner.work)
	}
}
