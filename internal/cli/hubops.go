package cli

// Hub lifecycle: the console's "更新程序 / 重启中心" buttons. The contract is
// frozen in docs/plan/2026-10-18-hub-self-update-contract.md (D4–D13); the
// interface it satisfies is web.HubOperations (internal/web/hubops.go).
//
// Three things about this file are deliberate and worth stating up front:
//
//  1. Everything that touches the OS — exec, hashing, the clock, the bind,
//     INVOCATION_ID, the release client, the agent restarter, exit — is a
//     field on hubOps. The rollback and failure branches are otherwise
//     unreachable from a test, and they are exactly the branches that must
//     never be wrong (a broken rollback bricks a hub).
//  2. After a failed exec the hub must keep serving (D12), but draining has
//     already shut the HTTP server and closed the listener. runServe therefore
//     runs a serve loop that re-binds and re-serves when this type signals
//     execFailed, instead of returning and letting the process die.
//  3. The write-lock probe is an optional seam (tryWriteLock). internal/web
//     keeps writeMutex package-private and server.go belongs to another
//     worker, so this process cannot read it yet; nil means "cannot tell" and
//     the restart proceeds with Busy=false — which is what D11 mandates
//     anyway, since a busy hub must never refuse a restart.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/zzjcool/homer/internal/cli/commands"
	"github.com/zzjcool/homer/internal/upgrade"
	"github.com/zzjcool/homer/internal/web"
)

// Hub update phases and error codes.
//
// These alias the exported web.HubPhase* / web.HubCode* constants, which are
// themselves aliases of the unexported ones in internal/web/hubops.go. The
// chain matters: web.HubStatus.Phase is what the web layer's draining rule
// keys on, so a literal spelled here could drift from the value the web layer
// compares against and silently disable draining. Referencing the exported
// alias makes that impossible.
const (
	hubPhaseIdle        = web.HubPhaseIdle
	hubPhaseChecking    = web.HubPhaseChecking
	hubPhaseDownloading = web.HubPhaseDownloading
	hubPhaseInstalling  = web.HubPhaseInstalling
	hubPhaseRestarting  = web.HubPhaseRestarting
	hubPhaseFailed      = web.HubPhaseFailed

	hubCodeUnsupported = web.HubCodeUnsupported
	hubCodeDevVersion  = web.HubCodeDevVersion
	hubCodeRunning     = web.HubCodeRunning
	hubCodeFailed      = web.HubCodeFailed
)

// Frozen user-facing strings (contract D7/D8/D13). The console renders `note`
// verbatim, so these are copy, not debug output.
const (
	hubMsgUnsupported = "此平台不支持自重启，请手动重启 homer serve"
	hubMsgDevVersion  = "这是源码构建的 hub，不能自动更新：请在源码目录重新构建并重启"
	hubMsgRunning     = "更新正在进行，完成后会自动重启"
	hubMsgUpToDate    = "已是最新版本"
	hubNoteDevBuild   = "这是源码构建的 hub，不能自动更新"
	hubNoteNoCheck    = "已关闭版本检查（HOMER_NO_VERSION_CHECK=1），不能自动更新"

	// D4 requires the two failure branches to stay distinguishable: this one
	// means the new binary IS on disk, it just never started.
	hubNoteInstalledNotRestarted = "新版本已装好，但中心没有重启；请在服务器上重新执行 homer serve"
	// ...and this one means nothing was written; the user can just retry.
	hubNoteNotWritten = "当前程序没有被替换，可以直接重试"
)

const (
	// D11's draining grace. Phase is the only signal the web layer can
	// observe (see drainAndExec), so in-flight writes get a window to see
	// phase=restarting and answer 503 instead of being cut mid-rename.
	hubRestartGrace = 200 * time.Millisecond
	// agentHub.Shutdown(≤3s): close 1001 "hub restarting".
	hubAgentShutdownWait = 3 * time.Second
	// httpServer.Shutdown(≤2s): never wait for a 12-minute pull.
	hubHTTPSShutdownWait = 2 * time.Second
	// Best-effort write-lock probe (D11). Losing it never blocks a restart.
	hubWriteProbeWait = 2 * time.Second
	// execVerdictWait bounds how long the serve loop waits for the restart
	// goroutine's failure verdict after the HTTP server has been drained.
	// syscall.Exec never returns on success, so this window only has to cover
	// the drain steps plus a failed exec (200ms grace + 3s agents + 2s HTTP +
	// 2s probe ≈ 7.2s) with room to spare. Exceeding it means that goroutine
	// died unexpectedly; the loop then re-binds instead of hanging with
	// nothing listening.
	execVerdictWait = 30 * time.Second
	// D5's budget for reloading local agents onto the new binary.
	hubAgentRestartWait = 10 * time.Second
	// D13: only a same-binary restart within this window is a crash-loop
	// candidate.
	hubBootRollbackWindow = 90 * time.Second
	// D4: HubStatus never blocks on the network; `latest` refreshes in the
	// background at most this often.
	hubLatestTTL = 10 * time.Minute
	// D10: the replacement image re-binds the port; give the kernel a moment
	// to release it after the exec closed the old listener fd.
	hubListenRetryCount = 20
	hubListenRetryDelay = 100 * time.Millisecond
	// Download staging path next to <self>, so the final rename is atomic and
	// on the same filesystem. Never write to <self> directly (D5).
	hubStagedSuffix = ".hub-new"
	// The single backup generation (D13). No history, no pinning.
	hubPreviousSuffix = ".prev"
	hubBootMarkerName = ".hub-boot"
)

// hubBootMarker is <home>/keys/.hub-boot. It exists only while a boot has not
// yet reached "ready" (listener bound), so finding one at startup means the
// previous process died trying. {sha256, startedAt, attempt} is D13's payload;
// pid is what lets the startup banner distinguish "原地重启（PID 未变）" from
// "由 supervisor 重新拉起" (D14).
type hubBootMarker struct {
	SHA256     string    `json:"sha256"`
	StartedAt  time.Time `json:"startedAt"`
	Attempt    int       `json:"attempt"`
	PID        int       `json:"pid,omitempty"`
	RolledBack bool      `json:"rolledBack,omitempty"`
}

// hubOps implements web.HubOperations. Construction is newHubOps; runServe
// owns the shutdown primitives it delegates to and the serve loop that
// recovers from a failed exec.
type hubOps struct {
	home      string
	self      string
	boundAddr string
	bootID    string

	// Seams. All of them have production defaults set by newHubOps; tests
	// replace the ones they care about.
	now             func() time.Time
	sha256          func(string) (string, error)
	exec            func(exe string, argv, env []string) error
	environ         func() []string
	invocationID    func() string
	reexecSupported func() bool
	sleep           func(time.Duration)
	exit            func(int)
	randomHex       func(int) string
	fetchLatest     func() (*upgrade.Latest, error)
	installArchive  func(tag, target string) error
	restartAgents   func(binary, home string) (string, error)
	agentShutdown   func(context.Context) error
	httpShutdown    func(context.Context) error
	// tryWriteLock reports whether the hub's write lock could be taken within
	// the deadline (releasing it immediately). nil = cannot tell.
	tryWriteLock func(context.Context) bool

	mu         sync.Mutex
	phase      string
	latest     string
	latestAt   time.Time
	refreshing bool
	downloaded int64
	total      int64
	note       string
	rolledBack bool
	// running spans StartHubUpdate's accepted chain; execPending spans an
	// accepted restart until the exec either fails or replaces the image.
	// Both keep a second click from stacking a second task.
	running     bool
	execPending bool
	// execFailed is signalled (buffer 1) when an accepted exec attempt
	// returned instead of replacing the image. runServe's serve loop consumes
	// it to re-bind and keep serving when there is no supervisor (D12).
	execFailed chan struct{}

	// bootNotice is the D14 banner line ("原地重启" vs "supervisor 拉起"),
	// empty on an ordinary cold start.
	bootNotice string
}

func newHubOps(home, boundAddr string) *hubOps {
	self, _ := upgrade.SelfPath() // empty on failure: reported when it matters
	return &hubOps{
		home:            home,
		self:            self,
		boundAddr:       boundAddr,
		bootID:          randomHexString(4), // 8 hex, one per process (D19)
		now:             time.Now,
		sha256:          sha256File,
		exec:            hubReexec,
		environ:         hubEnviron,
		invocationID:    func() string { return os.Getenv("INVOCATION_ID") },
		reexecSupported: hubReexecSupported,
		sleep:           time.Sleep,
		exit:            os.Exit,
		randomHex:       randomHexString,
		fetchLatest: func() (*upgrade.Latest, error) {
			return upgrade.NewClient(os.Getenv("HOMER_INSTALL_BASE_URL")).FetchLatest()
		},
		installArchive: func(tag, target string) error {
			return upgrade.NewDownloader(os.Getenv("HOMER_INSTALL_BASE_URL")).InstallArchive(tag, target)
		},
		restartAgents: commands.RestartRunningAgents,
		// The write-lock probe is what makes HubRestartAccepted.Busy truthful:
		// without it a restart during a long pull answers Busy=false and the
		// console cannot tell the user a sync was interrupted (contract D11).
		tryWriteLock:  web.ProbeWriteLock,
		agentShutdown: func(context.Context) error { return nil },
		httpShutdown:  func(context.Context) error { return nil },
		phase:         hubPhaseIdle,
		execFailed:    make(chan struct{}, 1),
	}
}

var _ web.HubOperations = (*hubOps)(nil)

// HubStatus reports the lifecycle from memory only. Per D4 the read path
// never performs network I/O: `latest` comes from the TTL cache, refreshed in
// the background by the first status read after it expires.
func (o *hubOps) HubStatus(context.Context) web.HubStatus {
	o.mu.Lock()
	canUpdate := o.canUpdateLocked()
	status := web.HubStatus{
		Version:    web.Version,
		BootID:     o.bootID,
		Phase:      o.phase,
		CanUpdate:  canUpdate,
		Latest:     o.latest,
		Downloaded: o.downloaded,
		Total:      o.total,
		Note:       o.note,
		RolledBack: o.rolledBack,
	}
	if !canUpdate && status.Note == "" {
		status.Note = o.updateBlockedNoteLocked()
	}
	stale := canUpdate && !o.refreshing && o.now().Sub(o.latestAt) >= hubLatestTTL
	if stale {
		o.refreshing = true
	}
	o.mu.Unlock()
	if stale {
		go o.refreshLatest()
	}
	return status
}

func (o *hubOps) refreshLatest() {
	defer func() {
		o.mu.Lock()
		o.refreshing = false
		o.mu.Unlock()
	}()
	latest, err := o.fetchLatest()
	o.mu.Lock()
	// Record the attempt either way: a failed check must not turn every
	// console poll into another request.
	o.latestAt = o.now()
	if err == nil && latest != nil {
		o.latest = latest.TagName
	}
	o.mu.Unlock()
}

// StartHubUpdate accepts the check→download→verify→install→restart chain and
// returns immediately (D5). The chain ends by exec'ing this process, so its
// outcome can never be reported through this response.
func (o *hubOps) StartHubUpdate(context.Context) (web.HubUpdateAccepted, error) {
	if err := o.acceptUpdate(); err != nil {
		return web.HubUpdateAccepted{}, err
	}
	go o.runUpdate()
	return web.HubUpdateAccepted{Phase: hubPhaseChecking, BootID: o.bootID}, nil
}

func (o *hubOps) acceptUpdate() error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.canUpdateLocked() {
		return hubError(409, hubCodeDevVersion, o.updateBlockedMessageLocked())
	}
	if strings.TrimSpace(o.self) == "" {
		// Pre-flight: without the running binary's path there is nothing to
		// replace. This is the one place the frozen 502 still reaches a
		// caller — every later failure happens after the 202 went out.
		return hubError(502, hubCodeFailed, "无法定位当前程序，无法自更新")
	}
	if o.running || (o.phase != hubPhaseIdle && o.phase != hubPhaseFailed) {
		return hubError(409, hubCodeRunning, hubMsgRunning)
	}
	o.running = true
	o.phase = hubPhaseChecking
	o.note = ""
	o.downloaded, o.total = 0, 0
	return nil
}

// runUpdate is the background half of StartHubUpdate. The order is frozen by
// D5: check → download+verify → install (backup, then rename) → reload local
// agents (best effort) → drain → exec.
func (o *hubOps) runUpdate() {
	defer o.finishUpdate()

	latest, err := o.fetchLatest()
	if err != nil {
		o.setPhase(hubPhaseFailed, "无法获取最新版本信息（离线或网络受限）："+err.Error()+"。"+hubNoteNotWritten+"。")
		return
	}
	o.mu.Lock()
	target := ""
	if latest != nil {
		o.latest = latest.TagName
		target = latest.TagName
	}
	o.latestAt = o.now()
	current := web.Version
	o.mu.Unlock()

	if target == "" {
		o.setPhase(hubPhaseFailed, "版本检查没有返回版本号。"+hubNoteNotWritten+"。")
		return
	}
	if !upgrade.IsNewer(target, current) {
		o.setPhase(hubPhaseIdle, hubMsgUpToDate+" "+target)
		return
	}

	o.setPhase(hubPhaseDownloading, "")
	// Download + checksum verification land NEXT TO the running binary, never
	// on it: InstallArchive verifies checksums.txt before it renames, and any
	// failure there must leave the running program untouched (D4 branch ①).
	staged := o.self + hubStagedSuffix
	if err := o.installArchive(target, staged); err != nil {
		removeHubStaging(staged)
		o.setPhase(hubPhaseFailed, "下载或校验失败："+err.Error()+"。"+hubNoteNotWritten+"。")
		return
	}

	o.setPhase(hubPhaseInstalling, "")
	if err := o.replaceSelf(staged); err != nil {
		removeHubStaging(staged)
		o.setPhase(hubPhaseFailed, "安装失败："+err.Error()+"。"+hubNoteNotWritten+"。")
		return
	}

	// D5: the upgrade path must reload the local agent, or it keeps running
	// the old inode. A pure restart does not touch agents: an old-inode agent
	// is equivalent to a not-yet-upgraded remote.
	agentsNote := o.restartAgentsBestEffort()

	o.mu.Lock()
	o.execPending = true
	o.mu.Unlock()
	o.setPhase(hubPhaseRestarting, "")
	if err := o.drainAndExec(); err != nil {
		o.execFailure(err, hubNoteInstalledNotRestarted+agentsNote)
	}
}

// setBoundAddr records the address this process actually bound. The re-exec
// argv is normalised onto it (D10).
func (o *hubOps) setBoundAddr(addr string) {
	o.mu.Lock()
	o.boundAddr = addr
	o.mu.Unlock()
}

// shutdownFuncs snapshots the primitives runServe installs for the current
// serve iteration. They are replaced when a failed exec forces a re-bind, so
// every reader takes a snapshot instead of touching the fields directly.
func (o *hubOps) shutdownFuncs() (func(context.Context) error, func(context.Context) error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.agentShutdown, o.httpShutdown
}

// setShutdown installs the current iteration's shutdown primitives.
func (o *hubOps) setShutdown(agent, http func(context.Context) error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.agentShutdown = agent
	o.httpShutdown = http
}

// replaceSelf performs D13's two renames: the running binary becomes
// <self>.prev first (so a crash loop has something to go back to), then the
// verified download takes its place.
func (o *hubOps) replaceSelf(staged string) error {
	previous := o.self + hubPreviousSuffix
	// Exactly one previous generation; no history, no pinning (D13).
	_ = os.Remove(previous)
	if err := os.Rename(o.self, previous); err != nil {
		return fmt.Errorf("无法备份当前程序到 %s：%w", previous, err)
	}
	if err := os.Rename(staged, o.self); err != nil {
		// Never leave the hub without an executable on disk: put the backup
		// back and report the failure instead.
		if restoreErr := os.Rename(previous, o.self); restoreErr != nil {
			return fmt.Errorf("替换程序失败且无法还原（原程序在 %s）：%v", previous, err)
		}
		return err
	}
	return nil
}

// restartAgentsBestEffort reloads running `homer agent` processes onto the
// binary just installed, bounded by D5's 10s budget. Off Linux the helper
// answers "需要手动重新启动 agent": that is an expected degradation and must
// not fail the update, so the message only becomes part of the note.
func (o *hubOps) restartAgentsBestEffort() string {
	type outcome struct {
		note string
		err  error
	}
	done := make(chan outcome, 1)
	go func() {
		note, err := o.restartAgents(o.self, o.home)
		done <- outcome{note: note, err: err}
	}()
	select {
	case result := <-done:
		if result.note != "" {
			log.Printf("homer serve: %s", result.note)
		}
		if result.err != nil {
			log.Printf("homer serve: 重启本机 agent 失败（不影响更新）：%v", result.err)
			return "；本机 agent 需要手动重启"
		}
		return ""
	case <-time.After(hubAgentRestartWait):
		log.Printf("homer serve: 重启本机 agent 超过 %s，继续重启中心", hubAgentRestartWait)
		return "；本机 agent 重启超时，需要手动重启"
	}
}

// RestartHub accepts a pure restart (no agent processes involved).
func (o *hubOps) RestartHub(ctx context.Context) (web.HubRestartAccepted, error) {
	if !o.reexecSupported() {
		// D7/D9: 501 and never exit — the hub keeps running on this platform.
		return web.HubRestartAccepted{}, hubError(501, hubCodeUnsupported, hubMsgUnsupported)
	}
	o.mu.Lock()
	switch {
	case o.running, o.phase == hubPhaseDownloading, o.phase == hubPhaseInstalling:
		// Exec'ing in the middle of a binary replacement is never allowed
		// (D5); a second update while one runs is a 409 (D4).
		o.mu.Unlock()
		return web.HubRestartAccepted{}, hubError(409, hubCodeRunning, hubMsgRunning)
	case o.execPending:
		// Already draining: answer 202 again instead of stacking a second
		// exec. The contract does not make this a 409 — a restart is
		// idempotent from the console's point of view.
		o.mu.Unlock()
		return web.HubRestartAccepted{RestartID: o.randomHex(8), BootID: o.bootID}, nil
	}
	o.execPending = true
	o.note = ""
	o.mu.Unlock()

	// D11 puts draining FIRST: publishing phase=restarting before the probe is
	// what lets in-flight writes see it and answer 503 instead of waiting out
	// the 2s window. It also makes the returned 202 truthful — by the time the
	// caller reads it, the hub is already draining.
	o.setPhase(hubPhaseRestarting, "")

	// Probe before answering so Busy is known when the 202 is written. Taking
	// the lock is best effort only; losing it never blocks the restart (D11).
	busy := o.writeBusy(ctx)

	go func() {
		if err := o.drainAndExec(); err != nil {
			o.execFailure(err, "")
		}
	}()
	return web.HubRestartAccepted{RestartID: o.randomHex(8), BootID: o.bootID, Busy: busy}, nil
}

func (o *hubOps) writeBusy(ctx context.Context) bool {
	if o.tryWriteLock == nil {
		return false
	}
	probeCtx, cancel := context.WithTimeout(ctx, hubWriteProbeWait)
	defer cancel()
	return !o.tryWriteLock(probeCtx)
}

// drainAndExec runs D11's fixed order: agent streams → HTTP server → (bounded,
// best-effort) write-lock probe → syscall.Exec. It must not hold the write
// lock when it execs, and it must never refuse the restart.
//
// A real success never returns — the process image is replaced. Anything it
// does return is a failure: a pre-exec failure (unknown self path,
// unsupported platform) or syscall.Exec's own error.
func (o *hubOps) drainAndExec() error {
	if !o.reexecSupported() {
		return errors.New(hubMsgUnsupported)
	}
	// Read the mutable fields under the lock: a failed exec can swap the
	// shutdown primitives while a download is still running.
	o.mu.Lock()
	self := o.self
	boundAddr := o.boundAddr
	agentShutdown, httpShutdown := o.agentShutdown, o.httpShutdown
	o.mu.Unlock()
	if strings.TrimSpace(self) == "" {
		return errors.New("无法定位当前程序，无法原地重启")
	}
	// The draining grace window. `internal/web` is another owner's file and
	// offers no draining flag, so by contract Phase == "restarting" is the
	// ONLY way the web layer learns a restart is in progress: the caller
	// publishes it, and this window gives in-flight writes time to see it and
	// answer 503 "restarting" instead of being cut mid-rename.
	o.sleep(hubRestartGrace)

	agentCtx, cancelAgent := context.WithTimeout(context.Background(), hubAgentShutdownWait)
	// close 1001 "hub restarting": agents fail fast and reconnect instead of
	// waiting out their ping timeout against a hub that is gone.
	_ = agentShutdown(agentCtx)
	cancelAgent()

	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), hubHTTPSShutdownWait)
	// Bounded on purpose: a pull may legitimately hold the hub for 12
	// minutes and must not hold the restart button hostage.
	_ = httpShutdown(httpCtx)
	cancelHTTP()

	// Best effort: wait up to 2s for an in-flight write to finish, then go
	// anyway. Holding the lock across Exec is forbidden, and "busy means
	// refuse" would make the button randomly useless (D11).
	if o.tryWriteLock != nil {
		probeCtx, cancelProbe := context.WithTimeout(context.Background(), hubWriteProbeWait)
		_ = o.tryWriteLock(probeCtx)
		cancelProbe()
	}

	return o.exec(self, normalizeReexecArgs(os.Args, boundAddr), o.environ())
}

// execFailure implements D12: without a supervisor the hub stays up and
// usable; with one, the half-stopped process is handed back to it.
//
// The 202 has already been answered by the time an exec can fail, so failures
// are surfaced as status (phase/note) rather than as a HubError — the
// console's bootId poll times out and D19's copy takes over. The only
// HubError-mapped failure happens before acceptance (unsupported platform,
// missing self path).
func (o *hubOps) execFailure(err error, note string) {
	log.Printf("homer serve: 原地重启失败（目标 %s）：%v", o.self, err)
	o.mu.Lock()
	o.execPending = false
	o.mu.Unlock()
	// Wake runServe's serve loop: with no supervisor it has to re-bind and
	// keep serving (D12). The supervisor branch below exits instead, so it
	// must not signal a recovery.
	if o.invocationID() != "" {
		// supervisor restart: do not leave behind a process whose listener
		// and agent streams are already closed while it still answers.
		o.setPhase(hubPhaseFailed, note+"（已交给 supervisor 重新拉起）："+err.Error())
		o.stopAndExit()
		return
	}
	select {
	case o.execFailed <- struct{}{}:
	default:
	}
	if note != "" {
		// Update branch ②: the new binary IS on disk, it just never started.
		// Phase stays failed so the console picks the "已装好、未重启" copy;
		// draining is keyed on phase==restarting, so it is off again.
		o.setPhase(hubPhaseFailed, note+"："+err.Error())
		return
	}
	// A pure restart failed with no supervisor: the hub MUST keep serving. A
	// drained hub is strictly worse than a failed restart.
	o.setPhase(hubPhaseIdle, "中心没有重启："+err.Error()+"。可以直接重试。")
}

func (o *hubOps) stopAndExit() {
	agentShutdown, httpShutdown := o.shutdownFuncs()
	agentCtx, cancelAgent := context.WithTimeout(context.Background(), time.Second)
	_ = agentShutdown(agentCtx)
	cancelAgent()
	httpCtx, cancelHTTP := context.WithTimeout(context.Background(), time.Second)
	_ = httpShutdown(httpCtx)
	cancelHTTP()
	o.exit(1)
}

// finishUpdate clears the single-flight guard even if the chain returned
// early; a stuck `running` would 409 every future click.
func (o *hubOps) finishUpdate() {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.running = false
	switch o.phase {
	case hubPhaseChecking, hubPhaseDownloading, hubPhaseInstalling:
		o.phase = hubPhaseIdle
	}
}

func (o *hubOps) setPhase(phase, note string) {
	o.mu.Lock()
	o.phase = phase
	if note != "" || phase == hubPhaseIdle {
		o.note = note
	}
	o.mu.Unlock()
}

func (o *hubOps) canUpdateLocked() bool {
	return o.versionUpdatable() && !hubVersionCheckDisabled()
}

// versionUpdatable implements D7: a source build ("dev", or anything that is
// not a release-shaped version) must never self-update — a click would
// DOWNGRADE it to a release binary, contradicting hubVersionIsNewer's frozen
// "dev is newer than any release" semantics. There is no force switch.
func (o *hubOps) versionUpdatable() bool {
	version := strings.TrimSpace(web.Version)
	return version != "" && version != "dev" && upgrade.IsParseableVersion(version)
}

func hubVersionCheckDisabled() bool { return os.Getenv("HOMER_NO_VERSION_CHECK") == "1" }

func (o *hubOps) updateBlockedNoteLocked() string {
	if hubVersionCheckDisabled() {
		return hubNoteNoCheck
	}
	return hubNoteDevBuild
}

func (o *hubOps) updateBlockedMessageLocked() string {
	if hubVersionCheckDisabled() {
		return hubNoteNoCheck
	}
	return hubMsgDevVersion
}

// resumeBoot implements D13's startup half: write the boot marker, decide
// whether the previous process died in a crash loop, and if so revert to
// <self>.prev and exec into it once.
//
// It returns without exec'ing in every other case. A successful rollback never
// returns: the exec'd process finds the RolledBack marker and reports
// HubStatus.RolledBack for itself.
func (o *hubOps) resumeBoot() {
	rollbackNeeded, rolledBackBoot := o.beginBoot()
	o.mu.Lock()
	o.rolledBack = rolledBackBoot
	o.mu.Unlock()
	if !rollbackNeeded {
		return
	}
	if err := o.rollback(); err != nil {
		// A failed rollback must not take the hub down: keep serving the
		// version that at least got this far.
		log.Printf("homer serve: 回滚到上一版程序失败（%s）：%v", o.self, err)
		return
	}
	// rollback returns only from a failed exec — the rename already happened,
	// so the binary on disk is now the old one even though this process keeps
	// running the new one.
	log.Printf("homer serve: 回滚后的程序没有重新执行起来，当前进程仍在运行新版本（%s）", o.self)
}

// beginBoot writes the marker and reports (rollbackRequired, thisBootIsRevert).
// Rollback needs all four conditions (D13), all of them read off the marker:
//
//	marker exists · now-startedAt < 90s · marker sha256 == self sha256 · attempt == 2
//
// "attempt == 2" means "this would be the second short-lived boot of the same
// bytes": the first boot writes attempt=1 and dies before reaching ready, the
// second records attempt=2 without reverting, and it is the THIRD boot that
// reverts. The sha256 comparison is what stops a double `systemctl restart` of
// a perfectly good binary from being read as a crash loop.
func (o *hubOps) beginBoot() (rollbackRequired, thisBootIsRevert bool) {
	selfSHA, shaErr := o.sha256(o.self)
	previous, hadMarker := readHubBootMarker(o.markerPath())
	now := o.now()
	attempt := 1
	if shaErr == nil && hadMarker && previous.SHA256 != "" && strings.EqualFold(previous.SHA256, selfSHA) {
		switch {
		case previous.RolledBack:
			// This boot IS the revert: report it, never roll back again.
			thisBootIsRevert = true
		case now.Sub(previous.StartedAt) < hubBootRollbackWindow:
			attempt = previous.Attempt + 1
			rollbackRequired = previous.Attempt == 2
		}
	}
	o.writeBootMarker(hubBootMarker{SHA256: selfSHA, StartedAt: now, Attempt: attempt, PID: os.Getpid()})
	if hadMarker {
		// D14: the banner must say which kind of restart this is. Same PID
		// means syscall.Exec kept the process; a different PID means a
		// supervisor started a new one.
		if previous.PID != 0 && previous.PID == os.Getpid() {
			o.bootNotice = "homer serve: 已原地重启（PID 未变），端口和会话保持原样。"
		} else if previous.PID != 0 {
			o.bootNotice = fmt.Sprintf("homer serve: 已由 supervisor 重新拉起（上一个进程 PID %d）。", previous.PID)
		}
	}
	return rollbackRequired, thisBootIsRevert
}

// rollback restores <self>.prev and execs into it once.
func (o *hubOps) rollback() error {
	previous := o.self + hubPreviousSuffix
	// Hash before the rename: afterwards the backup no longer exists under
	// its own name.
	previousSHA, err := o.sha256(previous)
	if err != nil {
		return fmt.Errorf("找不到上一版程序（%s）：%w", previous, err)
	}
	if err := os.Rename(previous, o.self); err != nil {
		return fmt.Errorf("还原上一版程序失败：%w", err)
	}
	o.rolledBack = true
	// The marker tells the reverted process that it booted by rolling back
	// (so the console can say so) and forces attempt back to 1: after a
	// revert only the supervisor's StartLimitBurst bounds the blast radius.
	o.writeBootMarker(hubBootMarker{
		SHA256:     previousSHA,
		StartedAt:  o.now(),
		Attempt:    1,
		PID:        os.Getpid(),
		RolledBack: true,
	})
	log.Printf("homer serve: 新版本启动未成功，正在回滚到上一版程序（%s）", o.self)
	return o.exec(o.self, normalizeReexecArgs(os.Args, o.boundAddr), o.environ())
}

// markReady deletes the boot marker: the listener is up, so this process is
// no longer a crash-loop candidate.
func (o *hubOps) markReady() {
	if err := os.Remove(o.markerPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("homer serve: 无法清除启动标记（%s）：%v", o.markerPath(), err)
	}
}

func (o *hubOps) markerPath() string {
	return filepath.Join(o.home, "keys", hubBootMarkerName)
}

func (o *hubOps) writeBootMarker(marker hubBootMarker) {
	path := o.markerPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		log.Printf("homer serve: 无法创建启动标记目录（%s）：%v", filepath.Dir(path), err)
		return
	}
	data, err := json.Marshal(marker)
	if err != nil {
		log.Printf("homer serve: 无法编码启动标记：%v", err)
		return
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Printf("homer serve: 无法写入启动标记（%s）：%v", path, err)
	}
}

func readHubBootMarker(path string) (hubBootMarker, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return hubBootMarker{}, false
	}
	var marker hubBootMarker
	if err := json.Unmarshal(data, &marker); err != nil {
		return hubBootMarker{}, false
	}
	return marker, true
}

// normalizeReexecArgs rewrites argv so the replacement image binds the address
// this process actually bound (D10): an existing --addr is overwritten, a
// missing one is appended, and everything else passes through untouched (the
// environment is inherited separately).
func normalizeReexecArgs(args []string, boundAddr string) []string {
	if strings.TrimSpace(boundAddr) == "" {
		return append([]string(nil), args...)
	}
	if len(args) == 0 {
		return []string{"homer", "serve", "--addr", boundAddr}
	}
	normalized := make([]string, 0, len(args)+2)
	normalized = append(normalized, args[0])
	replaced := false
	for index := 1; index < len(args); index++ {
		arg := args[index]
		if arg == "--" {
			// Everything after -- is positional; serve takes none, so leave
			// that tail alone rather than change what it means. The address
			// still has to be normalised, so it goes BEFORE the separator.
			if !replaced {
				normalized = append(normalized, "--addr", boundAddr)
				replaced = true
			}
			normalized = append(normalized, args[index:]...)
			break
		}
		switch {
		case arg == "--addr":
			normalized = append(normalized, "--addr", boundAddr)
			if index+1 < len(args) {
				index++
			}
			replaced = true
		case strings.HasPrefix(arg, "--addr="):
			normalized = append(normalized, "--addr="+boundAddr)
			replaced = true
		default:
			normalized = append(normalized, arg)
		}
	}
	if !replaced {
		normalized = append(normalized, "--addr", boundAddr)
	}
	return normalized
}

// listenHub binds the serve address, retrying on EADDRINUSE for up to
// 20×100ms (D10). After an in-place Exec the old listener fd is closed by the
// exec but the kernel can hold the port for a moment; a single-shot bind would
// turn a successful restart into "监听失败: address already in use".
func listenHub(addr string) (net.Listener, error) {
	return listenHubWith(net.Listen, time.Sleep, addr)
}

func listenHubWith(listen func(string, string) (net.Listener, error), sleep func(time.Duration), addr string) (net.Listener, error) {
	var lastErr error
	for attempt := 0; attempt < hubListenRetryCount; attempt++ {
		listener, err := listen("tcp", addr)
		if err == nil {
			return listener, nil
		}
		if !hubAddrInUse(err) {
			// A real conflict (bad port, permission) is reported at once;
			// only the release race is worth waiting for.
			return nil, err
		}
		lastErr = err
		sleep(hubListenRetryDelay)
	}
	return nil, lastErr
}

// hubAddrInUse reports whether bind failed because the port is still held.
func hubAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	// Some paths surface the bind error as a string-only error rather than a
	// syscall.Errno.
	return strings.Contains(strings.ToLower(err.Error()), "address already in use")
}

// hubError builds the frozen HubError shape (D8): the web layer maps it
// straight onto the HTTP status and code it carries.
func hubError(status int, code, message string) error {
	return &web.HubError{Status: status, Code: code, Message: message}
}

func removeHubStaging(staged string) {
	_ = os.Remove(staged)
	_ = os.Remove(staged + ".homer-new")
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func randomHexString(size int) string {
	if size <= 0 {
		return ""
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		// The bootId is a console-side comparison token, not a secret; a
		// timestamp is a poor but working stand-in, and panicking here would
		// take `homer serve` down because of the entropy pool.
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}
