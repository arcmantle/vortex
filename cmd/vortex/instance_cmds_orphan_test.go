package main

import (
	"fmt"
	"net"
	"strings"
	"testing"

	"arcmantle/vortex/internal/instance"
)

func TestRunBareModeReportsOrphanedSingleton(t *testing.T) {
	oldTryLock := tryLockInstance
	oldGetMetadata := getInstanceMetadata
	oldShowUI := showInstanceUI
	t.Cleanup(func() {
		tryLockInstance = oldTryLock
		getInstanceMetadata = oldGetMetadata
		showInstanceUI = oldShowUI
	})

	tryLockInstance = func(identity instance.Identity) (net.Listener, bool, error) {
		if identity.Name != bareInstanceName {
			t.Fatalf("unexpected identity %q", identity.Name)
		}
		return nil, false, nil
	}
	getInstanceMetadata = func(name string) (instance.Metadata, error) {
		return instance.Metadata{}, fmt.Errorf("%w %q", instance.ErrMetadataNotFound, name)
	}
	showInstanceUI = func(identity instance.Identity) error {
		return fmt.Errorf("existing instance %q returned 401 Unauthorized", identity.DisplayName)
	}

	err := runBareMode(cliOptions{})
	if err == nil {
		t.Fatal("expected runBareMode to fail")
	}
	if !strings.Contains(err.Error(), "registry metadata is missing") {
		t.Fatalf("expected missing-registry error, got %v", err)
	}
}

func TestRunInstancesCommandShowsOrphanedBareInstance(t *testing.T) {
	oldTryLock := tryLockInstance
	oldGetMetadata := getInstanceMetadata
	oldListMetadata := listInstanceMetadata
	t.Cleanup(func() {
		tryLockInstance = oldTryLock
		getInstanceMetadata = oldGetMetadata
		listInstanceMetadata = oldListMetadata
	})

	listInstanceMetadata = func() ([]instance.Metadata, error) {
		return nil, nil
	}
	tryLockInstance = func(identity instance.Identity) (net.Listener, bool, error) {
		if identity.Name != bareInstanceName {
			t.Fatalf("unexpected identity %q", identity.Name)
		}
		return nil, false, nil
	}
	getInstanceMetadata = func(name string) (instance.Metadata, error) {
		return instance.Metadata{}, fmt.Errorf("%w %q", instance.ErrMetadataNotFound, name)
	}

	output := captureStdout(t, func() {
		if err := runInstancesCommandWithOptions(instancesCommandOptions{}); err != nil {
			t.Fatalf("runInstancesCommandWithOptions() error = %v", err)
		}
	})

	if strings.Contains(output, "No running Vortex instances.") {
		t.Fatalf("expected orphaned instance output, got %q", output)
	}
	if !strings.Contains(output, "vortex") || !strings.Contains(output, "registry metadata is missing") {
		t.Fatalf("expected orphaned instance details, got %q", output)
	}
}
