//go:build !darwin

package main

import (
	"sync"
	"testing"
	"time"
)

func TestUIThreadRunnerPostAfterCloseDoesNotPanic(t *testing.T) {
	runner := newUIThreadRunner()
	runner.Close()

	// Post after Close must not panic.
	runner.Post(func() {
		t.Fatal("should not execute after Close")
	})
}

func TestUIThreadRunnerCloseIsIdempotent(t *testing.T) {
	runner := newUIThreadRunner()
	runner.Close()
	runner.Close() // second close must not panic
}

func TestUIThreadRunnerPostExecutesOnLockedThread(t *testing.T) {
	runner := newUIThreadRunner()
	defer runner.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	executed := false
	runner.Post(func() {
		executed = true
		wg.Done()
	})

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		if !executed {
			t.Fatal("Post callback was not executed")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Post callback")
	}
}

func TestUIThreadRunnerConcurrentPostAndClose(t *testing.T) {
	// Hammer Post and Close concurrently to verify no panics.
	for i := 0; i < 50; i++ {
		runner := newUIThreadRunner()
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			runner.Post(func() {})
		}()
		go func() {
			defer wg.Done()
			runner.Close()
		}()
		wg.Wait()
	}
}
