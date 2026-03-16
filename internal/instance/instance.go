// Package instance implements a single-instance lock using a TCP listener on a
// well-known loopback port. If the port is already bound the caller should
// forward its arguments to the existing instance instead of starting a new one.
package instance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"time"
)

const defaultHandoffPort = 7371

// HandoffPort is the port used for the single-instance handoff channel.
var HandoffPort = defaultHandoffPort

// HandoffPayload is the JSON body sent to an already-running instance.
type HandoffPayload struct {
	Args       []string `json:"args"`
	ConfigFile string   `json:"config_file,omitempty"`
}

// TryLock attempts to bind the handoff port. Returns (listener, true, nil) if
// this process won the lock, or (nil, false, nil) if another instance is
// already running.
func TryLock() (net.Listener, bool, error) {
	l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", HandoffPort))
	if err != nil {
		if isAddrInUse(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return l, true, nil
}

// Forward sends args to the running instance and waits for an acknowledgement.
func Forward(configFile string, args []string) error {
	payload, err := json.Marshal(HandoffPayload{ConfigFile: configFile, Args: args})
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/handoff", HandoffPort)
	resp, err := client.Post(url, "application/json", bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("could not reach existing instance: %w", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("existing instance returned %s", resp.Status)
	}
	return nil
}

// ServeHandoff starts an HTTP server on the instance-lock listener that accepts
// POST /handoff. This makes the lock port double as the handoff channel.
func ServeHandoff(l net.Listener, handler func(configFile string, args []string)) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /handoff", func(w http.ResponseWriter, r *http.Request) {
		var payload HandoffPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if handler != nil {
			go handler(payload.ConfigFile, payload.Args)
		}
		w.WriteHeader(http.StatusOK)
	})
	go http.Serve(l, mux) //nolint:errcheck
}

func isAddrInUse(err error) bool {
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		var errno syscall.Errno
		if errors.As(opErr.Err, &errno) {
			// EADDRINUSE on Unix, WSAEADDRINUSE (10048) on Windows.
			return errno == syscall.EADDRINUSE || errno == 10048
		}
	}
	return false
}
