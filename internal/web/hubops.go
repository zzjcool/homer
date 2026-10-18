package web

import "context"

// HubOperations is the hub's own lifecycle, as seen by the console: report
// status, self-update from the release channel, and restart in place.
//
// It is deliberately a ServeOptions field instead of an optional capability
// discovered by type assertion on AgentsSource: hub lifecycle has nothing to
// do with machines or syncing, so hanging it off AgentsSource would silently
// oblige every implementation (including test stubs and embedded servers) to
// carry hub duties. Nil means "this build cannot do it" and every /api/hub*
// route answers 501 hub-disabled — the same nil-keeps-legacy-behaviour
// precedent as Plugins and Overrides.
//
// The implementation lives in the cli package (which already imports web, so
// web must never import cli). All three methods are cheap for the caller:
// HubStatus must not perform network I/O.
type HubOperations interface {
	// HubStatus reports the current phase, versions and progress. It is
	// read from memory and must never block on the network: the console
	// polls it while a multi-minute download is running.
	HubStatus(ctx context.Context) HubStatus
	// StartHubUpdate begins the check→download→verify→install→restart
	// sequence and returns as soon as it is accepted. The restart happens
	// in the background because a successful update ends with the process
	// exec'ing itself, which can never be reported through this response.
	StartHubUpdate(ctx context.Context) (HubUpdateAccepted, error)
	// RestartHub accepts a restart and returns before the process is
	// replaced. The caller confirms success by watching /api/health's
	// bootId change.
	RestartHub(ctx context.Context) (HubRestartAccepted, error)
}

// HubStatus is the console's whole view of the hub's own lifecycle. Phase is
// one of the hubPhase* constants. Progress fields are only meaningful while
// downloading (Total is 0 when the server did not send a length).
type HubStatus struct {
	Version    string `json:"version"`
	BootID     string `json:"bootId"`
	Phase      string `json:"phase"`
	CanUpdate  bool   `json:"canUpdate"`
	Latest     string `json:"latest,omitempty"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
	Note       string `json:"note,omitempty"`
	// RolledBack is true when this process booted by reverting to the
	// previous binary after the new one failed to start. The console says
	// so instead of quietly showing the old version.
	RolledBack bool `json:"rolledBack,omitempty"`
}

// Hub update phases. "idle" is both the initial state and the resting state
// after a finished attempt; Note carries the human explanation in that case
// ("已是最新版本 v1.5.0").
const (
	hubPhaseIdle        = "idle"
	hubPhaseChecking    = "checking"
	hubPhaseDownloading = "downloading"
	hubPhaseInstalling  = "installing"
	hubPhaseRestarting  = "restarting"
	hubPhaseFailed      = "failed"
)

// Exported phase names for implementations outside this package (the cli
// package builds the HubOperations implementation but cannot see unexported
// identifiers). They are aliases, not copies: a change here changes the
// internal constants too, so the value can never drift between the producer
// and the consumer of HubStatus.Phase — which matters because the draining
// rule keys on PhaseRestarting.
const (
	HubPhaseIdle        = hubPhaseIdle
	HubPhaseChecking    = hubPhaseChecking
	HubPhaseDownloading = hubPhaseDownloading
	HubPhaseInstalling  = hubPhaseInstalling
	HubPhaseRestarting  = hubPhaseRestarting
	HubPhaseFailed      = hubPhaseFailed
)

// HubUpdateAccepted is the 202 body for POST /api/hub/upgrade.
type HubUpdateAccepted struct {
	Phase  string `json:"phase"`
	BootID string `json:"bootId"`
}

// HubRestartAccepted is the 202 body for POST /api/hub/restart. Busy means a
// write was holding the hub's global write lock when the restart was
// accepted: the console tells the user a sync was interrupted rather than
// pretending nothing was running.
type HubRestartAccepted struct {
	RestartID string `json:"restartId"`
	BootID    string `json:"bootId"`
	Busy      bool   `json:"busy"`
}

// HubError is an error with an explicit HTTP mapping, constructed by the cli
// implementation and rendered by the web layer. Any other error falls through
// to writeErrorValue's existing 500/502 mapping, so the error envelope stays
// single-sourced.
type HubError struct {
	Status  int
	Code    string
	Message string
	Details []string
}

func (e *HubError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// Hub error codes. Frozen by docs/plan/2026-10-18-hub-self-update-contract.md.
const (
	hubCodeDisabled    = "hub-disabled"
	hubCodeUnsupported = "hub-restart-unsupported"
	hubCodeDevVersion  = "hub-dev-version"
	hubCodeUpToDate    = "hub-up-to-date"
	hubCodeRunning     = "hub-update-running"
	hubCodeFailed      = "hub-update-failed"
	hubCodeRestarting  = "restarting"
)

// Exported error codes, for the same reason as the phase aliases above: the
// cli package constructs HubError values and must not spell the codes as
// string literals, which would let a rename in one place silently produce an
// unmapped code in the console.
const (
	HubCodeDisabled    = hubCodeDisabled
	HubCodeUnsupported = hubCodeUnsupported
	HubCodeDevVersion  = hubCodeDevVersion
	HubCodeUpToDate    = hubCodeUpToDate
	HubCodeRunning     = hubCodeRunning
	HubCodeFailed      = hubCodeFailed
	HubCodeRestarting  = hubCodeRestarting
)
