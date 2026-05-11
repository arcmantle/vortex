package instance

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestServeHandoffRejectsMissingTokenWhenRegistryMatches(t *testing.T) {
	useTempRegistryHome(t)

	identity := Identity{Name: "vortex", DisplayName: "vortex"}
	const sessionToken = "session-token"
	writeRegistryMetadata(t, identity, sessionToken)

	handled := make(chan HandoffPayload, 1)
	listener, url := startTestHandoffServer(t, identity, sessionToken, func(payload HandoffPayload) {
		handled <- payload
	})
	defer listener.Close()

	status := postHandoffStatus(t, url, HandoffPayload{Name: identity.Name, Action: handoffActionQuit})
	if status != http.StatusUnauthorized {
		t.Fatalf("expected %d, got %d", http.StatusUnauthorized, status)
	}
	select {
	case payload := <-handled:
		t.Fatalf("expected request rejection, got payload %+v", payload)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestServeHandoffAllowsMissingTokenWithoutRegistryToken(t *testing.T) {
	useTempRegistryHome(t)

	identity := Identity{Name: "vortex", DisplayName: "vortex"}
	const sessionToken = "session-token"

	handled := make(chan HandoffPayload, 1)
	listener, url := startTestHandoffServer(t, identity, sessionToken, func(payload HandoffPayload) {
		handled <- payload
	})
	defer listener.Close()

	status := postHandoffStatus(t, url, HandoffPayload{Name: identity.Name, Action: handoffActionQuit})
	if status != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, status)
	}
	select {
	case payload := <-handled:
		if payload.Action != handoffActionQuit {
			t.Fatalf("expected action %q, got %q", handoffActionQuit, payload.Action)
		}
	case <-time.After(time.Second):
		t.Fatal("expected handoff handler to receive payload")
	}
}

func useTempRegistryHome(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, ".cache"))
}

func writeRegistryMetadata(t *testing.T, identity Identity, token string) {
	t.Helper()
	dir, err := registryDir()
	if err != nil {
		t.Fatalf("registryDir: %v", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := writeMetadataFile(dir, Metadata{Name: identity.Name, DisplayName: identity.DisplayName, Token: token}); err != nil {
		t.Fatalf("writeMetadataFile: %v", err)
	}
}

func startTestHandoffServer(t *testing.T, identity Identity, token string, handler func(HandoffPayload)) (net.Listener, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	ServeHandoff(listener, identity, token, handler)
	return listener, "http://" + listener.Addr().String() + "/handoff"
}

func postHandoffStatus(t *testing.T, url string, payload HandoffPayload) int {
	t.Helper()
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Post: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
