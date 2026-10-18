//go:build linux || darwin

package cli

import (
	"os"
	"syscall"
)

// hubReexecSupported reports whether this platform can replace the running
// process image in place (D9). The restart endpoint answers 501
// hub-restart-unsupported everywhere else, and never exits.
func hubReexecSupported() bool { return true }

// hubReexec replaces this process with the binary at exe, keeping the PID.
//
// This is the same primitive internal/agentd/reexec_unix.go uses for the
// agent. It is deliberately not "start a child and exit": homer-serve may be
// Type=simple, Restart=always or Restart=no — the unit is not in the
// repository — so only an in-place exec behaves identically under all three,
// and never fights a supervisor over who owns the port.
//
// On success it never returns; the caller's process image is gone.
func hubReexec(exe string, argv, env []string) error {
	// The listener fd is closed by the exec (Go opens sockets with
	// SOCK_CLOEXEC), so the new image binds the port itself; see
	// listenWithRebindRetry for the port-release race.
	return syscall.Exec(exe, argv, env)
}

// hubEnviron is the environment inherited by the replacement image.
func hubEnviron() []string { return os.Environ() }
