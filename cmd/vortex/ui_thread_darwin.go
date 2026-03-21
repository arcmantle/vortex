//go:build darwin

package main

type uiThreadRunner struct{}

func newUIThreadRunner() *uiThreadRunner {
	return nil
}

func (runner *uiThreadRunner) Post(fn func()) {}