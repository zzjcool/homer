package cli

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/zzjcool/homer/internal/upgrade"
	"github.com/zzjcool/homer/internal/web"
)

// testHubOps builds a hubOps whose every OS-facing seam is a recording fake,
// so the rollback and failure branches are reachable. web.Version is pinned to
// a release-shaped value (D7 rejects "dev") and restored afterwards.
func testHubOps(t *testing.T, version string) (*hubOps, *hubFakes) {
	t.Helper()
	oldVersion := web.Version
	web.Version = version
	t.Cleanup(func() { web.Version = oldVersion })
	t.Setenv("HOMER_NO_VERSION_CHECK", "")
	t.Setenv("INVOCATION_ID", "")

	home := t.TempDir()
	ops := newHubOps(home, "127.0.0.1:7760")
	fakes := &hubFakes{ops: ops, home: home, exitCode: make(chan int, 1)}
	ops.self = filepath.Join(home, "homer")
	if err := os.WriteFile(ops.self, []byte("v1.4.0-binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	ops.now = func() time.Time { return fakes.now }
	ops.sha256 = fakes.sha256
	ops.exec = fakes.exec
	ops.environ = func() []string { return []string{"PATH=/usr/bin"} }
	ops.invocationID = func() string { return fakes.invocationID }
	ops.reexecSupported = func() bool { return fakes.supported }
	ops.sleep = func(time.Duration) {} // no 200ms/backoff waits in tests
	ops.exit = func(code int) {
		// Buffered so a faked exit is observable without blocking, and safe to
		// read from the test goroutine while the restart goroutine is running.
		select {
		case fakes.exitCode <- code:
		default:
		}
	}
	ops.randomHex = func(size int) string { return strings.Repeat("ab", size) }
	ops.fetchLatest = fakes.fetchLatest
	ops.installArchive = fakes.installArchive
	ops.restartAgents = fakes.restartAgents
	ops.agentShutdown = fakes.agentShutdown
	ops.httpShutdown = fakes.httpShutdown
	fakes.now = time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	return ops, fakes
}

type hubFakes struct {
	ops          *hubOps
	home         string
	now          time.Time
	invocationID string
	supported    bool
	// exitCode receives the code passed to the faked os.Exit.
	exitCode chan int

	mu             sync.Mutex
	execCalls      []hubExecCall
	execErr        error
	agentShutdowns int
	httpShutdowns  int
	agentRestarts  int
	installed      []string
	latest         *upgrade.Latest
	fetchErr       error
	installErr     error
	agentNote      string
	agentErr       error
	// execOverride and fetchOverride let a single test replace the fake
	// method's behaviour without a second fake type.
	execOverride  func(string, []string, []string) error
	fetchOverride func() (*upgrade.Latest, error)
	// hashes overrides the sha256 seam; absent, real file hashing is used so
	// tests exercise the real comparison.
	hashes map[string]string
}

type hubExecCall struct {
	exe  string
	argv []string
	env  []string
}

func (f *hubFakes) exec(exe string, argv, env []string) error {
	f.mu.Lock()
	f.execCalls = append(f.execCalls, hubExecCall{exe: exe, argv: append([]string(nil), argv...), env: append([]string(nil), env...)})
	override := f.execOverride
	f.mu.Unlock()
	if override != nil {
		return override(exe, argv, env)
	}
	return f.execErr
}

func (f *hubFakes) sha256(path string) (string, error) {
	if hash, ok := f.hashes[path]; ok {
		return hash, nil
	}
	return sha256File(path)
}

func (f *hubFakes) fetchLatest() (*upgrade.Latest, error) {
	if f.fetchOverride != nil {
		return f.fetchOverride()
	}
	return f.latest, f.fetchErr
}

func (f *hubFakes) installArchive(tag, target string) error {
	if f.installErr != nil {
		return f.installErr
	}
	f.installed = append(f.installed, tag+"->"+target)
	return os.WriteFile(target, []byte("v1.5.0-binary"), 0o755)
}

func (f *hubFakes) restartAgents(string, string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentRestarts++
	return f.agentNote, f.agentErr
}

func (f *hubFakes) agentShutdown(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.agentShutdowns++
	return nil
}

func (f *hubFakes) httpShutdown(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.httpShutdowns++
	return nil
}

// waitExecSettled blocks until execFailure has published the recovery signal,
// so assertions never race the background goroutine.
func (f *hubFakes) waitExecSettled(t *testing.T) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-f.ops.execFailed:
			return
		case <-deadline:
			t.Fatal("exec failure never signalled recovery")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// D4: the six phases are the whole state machine, and a second POST /upgrade
// while one runs is 409 hub-update-running.
func TestHubUpdateStateMachineRejectsConcurrentUpdate(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.latest = &upgrade.Latest{TagName: "v1.5.0"}

	accepted, err := ops.StartHubUpdate(context.Background())
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	if accepted.Phase != hubPhaseChecking || accepted.BootID != ops.bootID {
		t.Fatalf("accepted = %+v", accepted)
	}

	_, err = ops.StartHubUpdate(context.Background())
	var hubErr *web.HubError
	if !errors.As(err, &hubErr) {
		t.Fatalf("second update error = %v (%T), want *web.HubError", err, err)
	}
	if hubErr.Status != 409 || hubErr.Code != hubCodeRunning {
		t.Fatalf("second update = %d %s, want 409 hub-update-running", hubErr.Status, hubErr.Code)
	}
	// Let the accepted chain settle before the temp dir is cleaned up.
	waitForUpdateDone(t, ops)
}

// D4/D5: downloading or installing forbids a concurrent restart — exec'ing in
// the middle of a binary replacement is never allowed.
func TestHubRestartRejectedWhileUpdateRuns(t *testing.T) {
	for _, phase := range []string{hubPhaseChecking, hubPhaseDownloading, hubPhaseInstalling} {
		ops, fakes := testHubOps(t, "v1.4.0")
		fakes.supported = true // the platform check runs before the phase check
		ops.mu.Lock()
		ops.running = true
		ops.phase = phase
		ops.mu.Unlock()

		_, err := ops.RestartHub(context.Background())
		var hubErr *web.HubError
		if !errors.As(err, &hubErr) || hubErr.Status != 409 || hubErr.Code != hubCodeRunning {
			t.Fatalf("phase %s: restart = %v, want 409 hub-update-running", phase, err)
		}
	}
}

// D7: a source build must never self-update — that would downgrade it to a
// release binary and contradict hubVersionIsNewer's frozen semantics.
func TestHubDevVersionCannotUpdate(t *testing.T) {
	for _, version := range []string{"dev", "", "not-a-version"} {
		ops, _ := testHubOps(t, version)
		status := ops.HubStatus(context.Background())
		if status.CanUpdate {
			t.Fatalf("version %q: canUpdate = true", version)
		}
		if status.Note == "" {
			t.Fatalf("version %q: dev build needs an explanatory note", version)
		}
		_, err := ops.StartHubUpdate(context.Background())
		var hubErr *web.HubError
		if !errors.As(err, &hubErr) || hubErr.Status != 409 || hubErr.Code != hubCodeDevVersion {
			t.Fatalf("version %q: update = %v, want 409 hub-dev-version", version, err)
		}
	}
}

// D7: HOMER_NO_VERSION_CHECK=1 blocks self-update the same way.
func TestHubVersionCheckDisabledBlocksUpdate(t *testing.T) {
	ops, _ := testHubOps(t, "v1.4.0")
	t.Setenv("HOMER_NO_VERSION_CHECK", "1")
	status := ops.HubStatus(context.Background())
	if status.CanUpdate {
		t.Fatal("canUpdate = true with HOMER_NO_VERSION_CHECK=1")
	}
	if !strings.Contains(status.Note, "HOMER_NO_VERSION_CHECK") {
		t.Fatalf("note = %q", status.Note)
	}
	if _, err := ops.StartHubUpdate(context.Background()); err == nil {
		t.Fatal("update accepted with version checks disabled")
	}
}

// D19: the bootId is generated once per process and every read returns it.
func TestHubBootIDIsStable(t *testing.T) {
	ops, _ := testHubOps(t, "v1.4.0")
	first := ops.HubStatus(context.Background()).BootID
	if len(first) != 8 {
		t.Fatalf("bootId = %q, want 8 hex chars", first)
	}
	for i := 0; i < 5; i++ {
		if got := ops.HubStatus(context.Background()).BootID; got != first {
			t.Fatalf("bootId changed: %q -> %q", first, got)
		}
	}
	other, _ := testHubOps(t, "v1.4.0")
	if other.HubStatus(context.Background()).BootID == first {
		t.Fatal("two processes must not share a bootId")
	}
}

// D10: the replacement image must bind the address this process actually
// bound, with the rest of argv passing through untouched.
func TestNormalizeReexecArgsReplacesAddr(t *testing.T) {
	cases := []struct {
		name  string
		args  []string
		bound string
		want  []string
	}{
		{
			name:  "missing addr is appended",
			args:  []string{"/usr/local/bin/homer", "serve", "--home", "/srv/homer"},
			bound: "0.0.0.0:7760",
			want:  []string{"/usr/local/bin/homer", "serve", "--home", "/srv/homer", "--addr", "0.0.0.0:7760"},
		},
		{
			name:  "separate addr is replaced with the bound address",
			args:  []string{"homer", "serve", "--addr", ":0", "--token", "t"},
			bound: "127.0.0.1:41234",
			want:  []string{"homer", "serve", "--addr", "127.0.0.1:41234", "--token", "t"},
		},
		{
			name:  "inline addr is replaced",
			args:  []string{"homer", "serve", "--addr=:0"},
			bound: "127.0.0.1:41234",
			want:  []string{"homer", "serve", "--addr=127.0.0.1:41234"},
		},
		{
			name:  "positional tail after -- is preserved",
			args:  []string{"homer", "serve", "--", "--addr"},
			bound: "127.0.0.1:41234",
			want:  []string{"homer", "serve", "--addr", "127.0.0.1:41234", "--", "--addr"},
		},
		{
			name:  "empty argv still yields a runnable serve command",
			args:  nil,
			bound: "127.0.0.1:41234",
			want:  []string{"homer", "serve", "--addr", "127.0.0.1:41234"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeReexecArgs(tc.args, tc.bound)
			if strings.Join(got, "\x00") != strings.Join(tc.want, "\x00") {
				t.Fatalf("normalizeReexecArgs(%q) = %q, want %q", tc.args, got, tc.want)
			}
			// The caller's slice must not be mutated: it is os.Args.
			if len(tc.args) > 0 && len(got) > 0 && tc.args[0] != got[0] {
				t.Fatalf("argv[0] rewritten: %q -> %q", tc.args[0], got[0])
			}
		})
	}
}

// D13: rollback needs all four conditions. This table walks each one,
// including the case that motivates the sha256 comparison — a double
// `systemctl restart` of a GOOD binary must not roll it back.
func TestHubRollbackRequiresAllFourConditions(t *testing.T) {
	const currentSHA = "aaaa1111"
	const previousSHA = "bbbb2222"
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name           string
		marker         *hubBootMarker
		wantRolled     bool
		wantRevertBoot bool
		wantAttempt    int
	}{
		{
			name: "no marker: ordinary cold start",
			// A cold start writes attempt 1.
			wantAttempt: 1,
		},
		{
			name:       "marker with different sha256: a good binary was restarted twice",
			marker:     &hubBootMarker{SHA256: "cccc3333", StartedAt: now.Add(-time.Second), Attempt: 1},
			wantRolled: false,
			// Different bytes: not a crash-loop candidate, counter resets.
			wantAttempt: 1,
		},
		{
			name:       "same sha256, attempt 1, inside the window: first short-lived boot is not yet a crash loop",
			marker:     &hubBootMarker{SHA256: currentSHA, StartedAt: now.Add(-time.Second), Attempt: 1},
			wantRolled: false,
			// A same-binary retry bumps the counter to 2; the boot AFTER that reverts.
			wantAttempt: 2,
		},
		{
			name:       "same sha256, attempt 2, inside the window: roll back",
			marker:     &hubBootMarker{SHA256: currentSHA, StartedAt: now.Add(-30 * time.Second), Attempt: 2},
			wantRolled: true,
		},
		{
			name:       "same sha256, attempt 2, but older than 90s: not a crash loop",
			marker:     &hubBootMarker{SHA256: currentSHA, StartedAt: now.Add(-hubBootRollbackWindow), Attempt: 2},
			wantRolled: false,
			// Outside the window the counter restarts at 1.
			wantAttempt: 1,
		},
		{
			name:       "already rolled back once: never a second time",
			marker:     &hubBootMarker{SHA256: currentSHA, StartedAt: now, Attempt: 2, RolledBack: true},
			wantRolled: false,
			// This process DID boot by reverting, so it reports that — but it
			// must not revert again, and the counter stays pinned at 1.
			wantRevertBoot: true,
			// The counter stays at 1: after a revert only the supervisor's
			// StartLimitBurst bounds the blast radius.
			wantAttempt: 1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ops, fakes := testHubOps(t, "v1.5.0")
			fakes.now = now
			fakes.supported = true
			previousPath := ops.self + hubPreviousSuffix
			if err := os.WriteFile(previousPath, []byte("v1.4.0-binary"), 0o755); err != nil {
				t.Fatal(err)
			}
			// The marker records the sha256 of whatever is on disk; make the
			// current binary hash to currentSHA.
			fakes.hashes = map[string]string{ops.self: currentSHA, previousPath: previousSHA}
			if tc.marker != nil {
				ops.writeBootMarker(*tc.marker)
			}

			ops.resumeBoot()

			if tc.wantRolled {
				if !ops.rolledBack {
					t.Fatal("expected a rollback")
				}
				if len(fakes.execCalls) != 1 {
					t.Fatalf("exec calls = %d, want exactly 1 rollback exec", len(fakes.execCalls))
				}
				call := fakes.execCalls[0]
				if call.exe != ops.self {
					t.Fatalf("rolled back into %q, want %q", call.exe, ops.self)
				}
				// The reverted binary must be the .prev content: the rename
				// replaced self with the old bytes.
				data, err := os.ReadFile(ops.self)
				if err != nil || string(data) != "v1.4.0-binary" {
					t.Fatalf("self after rollback = %q (%v)", data, err)
				}
				marker, ok := readHubBootMarker(ops.markerPath())
				if !ok || !marker.RolledBack || marker.Attempt != 1 {
					t.Fatalf("post-rollback marker = %+v ok=%v, want rolledBack with attempt 1", marker, ok)
				}
				// The replacement must bind the same address (D10).
				if !strings.Contains(strings.Join(call.argv, " "), "--addr 127.0.0.1:7760") {
					t.Fatalf("rollback argv = %q", call.argv)
				}
				return
			}
			if ops.rolledBack != tc.wantRevertBoot {
				t.Fatalf("rolledBack = %v, want %v (exec calls=%d)", ops.rolledBack, tc.wantRevertBoot, len(fakes.execCalls))
			}
			if len(fakes.execCalls) != 0 {
				t.Fatalf("exec calls = %d, want none: a non-required rollback must not exec", len(fakes.execCalls))
			}
			if _, err := os.Stat(previousPath); err != nil {
				t.Fatalf("a non-rollback boot must leave .prev alone: %v", err)
			}
			marker, ok := readHubBootMarker(ops.markerPath())
			if !ok {
				t.Fatal("boot marker must be written at startup")
			}
			if marker.SHA256 != currentSHA {
				t.Fatalf("marker sha256 = %q, want %q", marker.SHA256, currentSHA)
			}
			if marker.Attempt != tc.wantAttempt {
				t.Fatalf("marker attempt = %d, want %d", marker.Attempt, tc.wantAttempt)
			}
		})
	}
}

// D13: ready deletes the marker, so a healthy running hub leaves nothing
// behind for the next boot to misread.
func TestHubMarkReadyRemovesBootMarker(t *testing.T) {
	ops, _ := testHubOps(t, "v1.4.0")
	ops.resumeBoot()
	if _, err := os.Stat(ops.markerPath()); err != nil {
		t.Fatalf("marker missing after boot: %v", err)
	}
	ops.markReady()
	if _, err := os.Stat(ops.markerPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("marker still present after ready: %v", err)
	}
	// Removing twice must stay quiet (serve loop rebuilds can call it).
	ops.markReady()
}

// D12 branch ①: exec failed and there is no supervisor. The hub must keep
// serving: no exit, draining cleared, phase back to idle with a note.
func TestHubExecFailureWithoutSupervisorKeepsServing(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.execErr = errors.New("exec format error")

	accepted, err := ops.RestartHub(context.Background())
	if err != nil {
		t.Fatalf("restart rejected: %v", err)
	}
	if accepted.RestartID == "" || accepted.BootID != ops.bootID {
		t.Fatalf("accepted = %+v", accepted)
	}
	fakes.waitExecSettled(t)

	select {
	case code := <-fakes.exitCode:
		t.Fatalf("no supervisor: the hub must not exit (exited %d)", code)
	default:
	}
	if len(fakes.execCalls) != 1 {
		t.Fatalf("exec calls = %d", len(fakes.execCalls))
	}
	status := ops.HubStatus(context.Background())
	if status.Phase != hubPhaseIdle {
		t.Fatalf("phase = %q, want idle (draining must be cleared)", status.Phase)
	}
	if !strings.Contains(status.Note, "exec format error") {
		t.Fatalf("note = %q, want the exec error", status.Note)
	}
	// D11 ordering: agent streams before the HTTP server, both before exec.
	if fakes.agentShutdowns != 1 || fakes.httpShutdowns != 1 {
		t.Fatalf("agentShutdown=%d httpShutdown=%d, want 1/1", fakes.agentShutdowns, fakes.httpShutdowns)
	}
}

// D12 branch ②: exec failed with a supervisor present. Hand the process back
// by stopping gracefully and exiting 1, so systemd restarts it.
func TestHubExecFailureWithSupervisorExits(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.invocationID = "9f2c"
	fakes.execErr = errors.New("permission denied")

	if _, err := ops.RestartHub(context.Background()); err != nil {
		t.Fatalf("restart rejected: %v", err)
	}
	// The supervisor branch never signals in-place recovery, so observe it
	// through the phase it records and the exit it requests.
	waitForPhase(t, ops, hubPhaseFailed)

	select {
	case code := <-fakes.exitCode:
		if code != 1 {
			t.Fatalf("exit(%d), want exit(1) for the supervisor branch", code)
		}
	default:
		t.Fatal("supervisor branch must exit(1) so the supervisor takes over")
	}
	// A supervisor will start a fresh process, so no in-place recovery.
	select {
	case <-ops.execFailed:
		t.Fatal("supervisor branch must not ask runServe to re-serve")
	default:
	}
}

// D12: the update branch that ends in a failed exec must say the new binary IS
// on disk — retrying the download would be wrong advice.
func TestHubUpdateExecFailureReportsInstalledNotRestarted(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.latest = &upgrade.Latest{TagName: "v1.5.0"}
	fakes.execErr = errors.New("text file busy")

	if _, err := ops.StartHubUpdate(context.Background()); err != nil {
		t.Fatalf("update rejected: %v", err)
	}
	fakes.waitExecSettled(t)

	status := ops.HubStatus(context.Background())
	if status.Phase != hubPhaseFailed {
		t.Fatalf("phase = %q, want failed", status.Phase)
	}
	if !strings.Contains(status.Note, "新版本已装好") {
		t.Fatalf("note = %q, want the 已装好-未重启 branch", status.Note)
	}
	if strings.Contains(status.Note, hubNoteNotWritten) {
		t.Fatalf("note = %q, must not claim the binary was not written", status.Note)
	}
}

// D5: a download/checksum failure happens before anything is written, so the
// note must send the user to retry, and .prev must not be touched.
func TestHubUpdateDownloadFailureIsRetryable(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.latest = &upgrade.Latest{TagName: "v1.5.0"}
	fakes.installErr = errors.New("checksum mismatch")

	if _, err := ops.StartHubUpdate(context.Background()); err != nil {
		t.Fatalf("update rejected: %v", err)
	}
	waitForPhase(t, ops, hubPhaseFailed)

	status := ops.HubStatus(context.Background())
	if !strings.Contains(status.Note, "checksum mismatch") {
		t.Fatalf("note = %q", status.Note)
	}
	if !strings.Contains(status.Note, hubNoteNotWritten) {
		t.Fatalf("note = %q, want the retryable branch", status.Note)
	}
	if len(fakes.execCalls) != 0 {
		t.Fatalf("exec calls = %d, want none", len(fakes.execCalls))
	}
	if fakes.agentRestarts != 0 {
		t.Fatal("a failed download must not restart agents")
	}
	if _, err := os.Stat(ops.self + hubPreviousSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf(".prev must not be created when the download failed: %v", err)
	}
	// Nothing may be left staged next to the binary.
	if _, err := os.Stat(ops.self + hubStagedSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staging file left behind: %v", err)
	}
}

// D5: already up to date is not an error — it is idle with a note.
func TestHubUpdateAlreadyLatestIsIdle(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.5.0")
	fakes.latest = &upgrade.Latest{TagName: "v1.5.0"}

	if _, err := ops.StartHubUpdate(context.Background()); err != nil {
		t.Fatalf("update rejected: %v", err)
	}
	waitForPhase(t, ops, hubPhaseIdle)

	status := ops.HubStatus(context.Background())
	if !strings.Contains(status.Note, hubMsgUpToDate) || !strings.Contains(status.Note, "v1.5.0") {
		t.Fatalf("note = %q, want the up-to-date message", status.Note)
	}
	if len(fakes.execCalls) != 0 || fakes.agentRestarts != 0 {
		t.Fatal("up-to-date must not install or restart anything")
	}
}

// D5: the upgrade path reloads local agents but a non-Linux "需要手动重启" is
// an expected degradation — it may not fail the update.
func TestHubUpdateAgentRestartDegradationDoesNotFail(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.latest = &upgrade.Latest{TagName: "v1.5.0"}
	fakes.agentNote = ""
	fakes.agentErr = errors.New("这个系统上需要手动重新启动 agent")
	// exec "succeeds" in the fake (returns nil) — the fake cannot replace the
	// process image, so signal completion as a failure-free settle instead.
	fakes.execErr = nil

	if _, err := ops.StartHubUpdate(context.Background()); err != nil {
		t.Fatalf("update rejected: %v", err)
	}
	waitForPhase(t, ops, hubPhaseRestarting)
	deadline := time.After(5 * time.Second)
	for len(fakes.execCalls) == 0 {
		select {
		case <-deadline:
			t.Fatal("exec never attempted")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	if fakes.agentRestarts != 1 {
		t.Fatalf("agent restarts = %d, want 1 on the upgrade path", fakes.agentRestarts)
	}
}

// D5: a pure restart must not touch agent processes at all.
func TestHubPureRestartDoesNotTouchAgents(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.execErr = errors.New("boom")

	if _, err := ops.RestartHub(context.Background()); err != nil {
		t.Fatalf("restart rejected: %v", err)
	}
	fakes.waitExecSettled(t)

	if fakes.agentRestarts != 0 {
		t.Fatalf("pure restart restarted %d agent(s)", fakes.agentRestarts)
	}
}

// D7/D9: unsupported platforms answer 501 and never exit.
func TestHubRestartUnsupportedPlatform(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = false

	_, err := ops.RestartHub(context.Background())
	var hubErr *web.HubError
	if !errors.As(err, &hubErr) || hubErr.Status != 501 || hubErr.Code != hubCodeUnsupported {
		t.Fatalf("restart = %v, want 501 hub-restart-unsupported", err)
	}
	select {
	case code := <-fakes.exitCode:
		t.Fatalf("unsupported platform must never exit the hub (exited %d)", code)
	default:
	}
	if len(fakes.execCalls) != 0 {
		t.Fatal("unsupported platform must not attempt an exec")
	}
}

// D11: the restart is published as phase=restarting before any drain work, so
// the web layer can answer 503 to in-flight writes.
func TestHubRestartPublishesDrainingPhase(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true

	// Observe the phase at the exact moment of the exec: D11 requires draining
	// to be published (and the drains to have run) before syscall.Exec.
	phaseAtExec := make(chan string, 1)
	agentAtExec := make(chan int, 1)
	httpAtExec := make(chan int, 1)
	fakes.execOverride = func(string, []string, []string) error {
		phaseAtExec <- ops.HubStatus(context.Background()).Phase
		fakes.mu.Lock()
		agentAtExec <- fakes.agentShutdowns
		httpAtExec <- fakes.httpShutdowns
		fakes.mu.Unlock()
		return errors.New("boom")
	}

	if _, err := ops.RestartHub(context.Background()); err != nil {
		t.Fatalf("restart rejected: %v", err)
	}

	select {
	case phase := <-phaseAtExec:
		if phase != hubPhaseRestarting {
			t.Fatalf("phase at exec = %q, want %q (draining must be published first)", phase, hubPhaseRestarting)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("exec never reached")
	}
	// D11 order: both drains happen BEFORE the exec.
	if agents := <-agentAtExec; agents != 1 {
		t.Fatalf("agent hub shut down %d times before exec, want 1", agents)
	}
	if httpDrains := <-httpAtExec; httpDrains != 1 {
		t.Fatalf("http server shut down %d times before exec, want 1", httpDrains)
	}
	fakes.waitExecSettled(t)

	// The 202 body was produced while draining was already published.
	if status := ops.HubStatus(context.Background()); status.Phase == hubPhaseRestarting {
		t.Fatal("a failed exec must clear draining")
	}
}

// D4/D5: a rejected restart must not touch the phase it refused to leave.
func TestHubRejectedRestartDoesNotPublishDraining(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	ops.mu.Lock()
	ops.running = true
	ops.phase = hubPhaseDownloading
	ops.mu.Unlock()

	if _, err := ops.RestartHub(context.Background()); err == nil {
		t.Fatal("restart accepted during an update")
	}
	if status := ops.HubStatus(context.Background()); status.Phase != hubPhaseDownloading {
		t.Fatalf("phase = %q, want %q untouched by the rejection", status.Phase, hubPhaseDownloading)
	}
	if len(fakes.execCalls) != 0 {
		t.Fatal("a rejected restart must not exec")
	}
}

// D11: Busy reflects the write-lock probe and never blocks the restart.
func TestHubRestartBusyProbeNeverRefuses(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.execErr = errors.New("boom")
	var probed atomic.Int32
	ops.tryWriteLock = func(context.Context) bool {
		probed.Add(1)
		return false // the lock is held by a 12-minute pull
	}

	accepted, err := ops.RestartHub(context.Background())
	if err != nil {
		t.Fatalf("restart rejected while busy: %v", err)
	}
	if !accepted.Busy {
		t.Fatal("busy = false, want the interrupted-sync warning")
	}
	if probed.Load() == 0 {
		t.Fatal("write lock was never probed")
	}
	fakes.waitExecSettled(t)
}

// D4: HubStatus must never perform network I/O on the read path; the TTL
// refresh happens in the background.
func TestHubStatusDoesNotBlockOnNetwork(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	release := make(chan struct{})
	fakes.fetchOverride = func() (*upgrade.Latest, error) {
		<-release
		return &upgrade.Latest{TagName: "v1.9.9"}, nil
	}

	done := make(chan web.HubStatus, 1)
	go func() { done <- ops.HubStatus(context.Background()) }()
	select {
	case status := <-done:
		if status.Latest != "" {
			t.Fatalf("latest = %q on a cold cache", status.Latest)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("HubStatus blocked on the network")
	}
	// Let the background refresh finish and confirm the cache picks it up.
	close(release)
	deadline := time.After(5 * time.Second)
	for {
		if status := ops.HubStatus(context.Background()); status.Latest == "v1.9.9" {
			return
		}
		select {
		case <-deadline:
			t.Fatal("background refresh never populated latest")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// D10: after an in-place Exec the port can be held for a moment; the bind
// retries the release race but fails fast on a real conflict.
func TestListenHubRetriesAddressInUse(t *testing.T) {
	var attempts int
	listen := func(network, addr string) (net.Listener, error) {
		attempts++
		if attempts <= 3 {
			return nil, &net.OpError{Op: "listen", Net: network, Err: syscall.EADDRINUSE}
		}
		return nil, errors.New("stop here")
	}
	var sleeps int
	sleep := func(time.Duration) { sleeps++ }
	if _, err := listenHubWith(listen, sleep, "127.0.0.1:0"); err == nil {
		t.Fatal("expected the final error to surface")
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4 (3 retries then success)", attempts)
	}
	if sleeps != 3 {
		t.Fatalf("sleeps = %d, want 3 backoffs", sleeps)
	}

	// A non-retryable error is reported immediately.
	attempts = 0
	if _, err := listenHubWith(func(string, string) (net.Listener, error) {
		attempts++
		return nil, errors.New("permission denied")
	}, sleep, "127.0.0.1:0"); err == nil {
		t.Fatal("expected the error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1 for a non-retryable error", attempts)
	}
}

// D13: staging and backup paths are siblings of the running binary, and the
// update never writes the binary in place.
func TestHubUpdateReplacesViaBackup(t *testing.T) {
	ops, fakes := testHubOps(t, "v1.4.0")
	fakes.supported = true
	fakes.latest = &upgrade.Latest{TagName: "v1.5.0"}
	fakes.execErr = errors.New("boom")

	original, err := os.ReadFile(ops.self)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ops.StartHubUpdate(context.Background()); err != nil {
		t.Fatalf("update rejected: %v", err)
	}
	fakes.waitExecSettled(t)

	installed, err := os.ReadFile(ops.self + hubPreviousSuffix)
	if err != nil {
		t.Fatalf(".prev missing: %v", err)
	}
	if string(installed) != string(original) {
		t.Fatalf(".prev = %q, want the original binary", installed)
	}
	current, err := os.ReadFile(ops.self)
	if err != nil || string(current) != "v1.5.0-binary" {
		t.Fatalf("self = %q (%v), want the new binary", current, err)
	}
	if len(fakes.installed) != 1 || !strings.HasSuffix(fakes.installed[0], hubStagedSuffix) {
		t.Fatalf("install target = %v, want the staging path (never self)", fakes.installed)
	}
	if strings.HasSuffix(fakes.installed[0], ops.self+hubStagedSuffix) == false {
		t.Fatalf("install target = %v", fakes.installed)
	}
}

// waitForUpdateDone blocks until the accepted chain cleared its single-flight
// guard. Tests need it before the temp home is removed: a download still in
// flight would recreate files under it.
func waitForUpdateDone(t *testing.T, ops *hubOps) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		ops.mu.Lock()
		running := ops.running
		ops.mu.Unlock()
		if !running {
			return
		}
		select {
		case <-deadline:
			t.Fatal("update chain never finished")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}

// waitForPhase polls the in-memory phase until it matches or the test fails.
func waitForPhase(t *testing.T, ops *hubOps, want string) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if ops.HubStatus(context.Background()).Phase == want {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("phase never became %q (now %q)", want, ops.HubStatus(context.Background()).Phase)
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
