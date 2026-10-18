package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestHubRestartReplacesProcessInPlace is the regression test for the bug the
// console's "仅重启" button shipped with: http.Server.Shutdown makes Serve
// return nil, and the serve loop used to treat a nil return as "we are done"
// and exit the process — on the main goroutine, before the restart goroutine
// reached syscall.Exec. The endpoint answered 202 and then the hub simply
// died, which reads as a successful restart in the UI until the bootId poll
// times out.
//
// The assertion that makes this test worth its cost is the PID: a passing
// restart keeps the same PID (contract D9's whole point — systemd's Restart=
// setting must not matter), while the pre-fix behaviour loses the process
// entirely. Checking bootId alone would also pass a supervisor restart, so
// both are asserted.
func TestHubRestartReplacesProcessInPlace(t *testing.T) {
	if _, err := os.Stat("/proc"); err != nil {
		t.Skip("needs /proc to observe the process id")
	}
	binary := binaryOf(t)
	home := t.TempDir()
	token := "restart-e2e-token"

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick port: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close()

	serve := exec.Command(binary, "serve", "--addr", addr, "--home", home)
	serve.Env = append(os.Environ(), "HOMER_HUB_TOKEN="+token, "HOMER_HOME="+home)
	var serveOut lockedBuffer
	serve.Stdout = &serveOut
	serve.Stderr = &serveOut
	if err := serve.Start(); err != nil {
		t.Fatalf("start hub: %v", err)
	}
	defer func() {
		_ = serve.Process.Kill()
		_, _ = serve.Process.Wait()
	}()

	base := "http://" + addr
	if !waitForHealth(t, base, 15*time.Second) {
		t.Fatalf("hub never became healthy:\n%s", serveOut.String())
	}
	before := healthBody(t, base)
	if before.BootID == "" {
		t.Fatalf("health has no bootId: %+v", before)
	}

	oldPID := serve.Process.Pid
	resp := authorizedRequest(t, http.MethodPost, base+"/api/hub/restart", token, "")
	if resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("restart returned %d, want 202: %s\n%s", resp.StatusCode, body, serveOut.String())
	}
	resp.Body.Close()

	// A successful in-place restart keeps the PID and changes the bootId. Poll
	// until the bootId differs, then assert the process identity.
	deadline := time.Now().Add(30 * time.Second)
	var after healthPayload
	for time.Now().Before(deadline) {
		if !processAlive(oldPID) {
			t.Fatalf("hub process %d exited during an in-place restart (the pre-fix hang: Serve returned nil and the serve loop returned 1)\n%s", oldPID, serveOut.String())
		}
		current, err := tryHealth(base)
		if err == nil && current.BootID != "" && current.BootID != before.BootID {
			after = current
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if after.BootID == "" {
		t.Fatalf("bootId never changed from %q after restart\n%s", before.BootID, serveOut.String())
	}
	if !processAlive(oldPID) {
		t.Fatalf("hub process %d exited after the restart\n%s", oldPID, serveOut.String())
	}
	if after.Version != before.Version {
		t.Fatalf("version changed across a pure restart: %q -> %q", before.Version, after.Version)
	}
}

type healthPayload struct {
	OK      bool   `json:"ok"`
	BootID  string `json:"bootId"`
	Version string `json:"version"`
}

func tryHealth(base string) (healthPayload, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(base + "/api/health")
	if err != nil {
		return healthPayload{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return healthPayload{}, fmt.Errorf("health returned %d", resp.StatusCode)
	}
	var payload healthPayload
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return healthPayload{}, err
	}
	return payload, nil
}

func healthBody(t *testing.T, base string) healthPayload {
	t.Helper()
	payload, err := tryHealth(base)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	return payload
}

func waitForHealth(t *testing.T, base string, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := tryHealth(base); err == nil {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func authorizedRequest(t *testing.T, method, url, token, body string) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = stringReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(request)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp
}

func stringReader(value string) io.Reader { return &stringReadCloser{remaining: value} }

type stringReadCloser struct{ remaining string }

func (r *stringReadCloser) Read(p []byte) (int, error) {
	if r.remaining == "" {
		return 0, io.EOF
	}
	n := copy(p, r.remaining)
	r.remaining = r.remaining[n:]
	return n, nil
}

// processAlive reports whether pid is still running. Signal 0 performs the
// permission and existence checks without delivering anything.
func processAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return process.Signal(syscall.Signal(0)) == nil
}

type lockedBuffer struct {
	data []byte
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *lockedBuffer) String() string { return string(b.data) }

// keep filepath imported for binaryOf-adjacent helpers in this package.
var _ = filepath.Join
