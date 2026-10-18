//go:build !linux && !darwin

package cli

import (
	"errors"
	"os"
)

// hubReexecSupported reports whether this platform can replace the running
// process image in place (D9). Off linux/darwin the restart endpoint answers
// 501 hub-restart-unsupported; it must never terminate the hub.
func hubReexecSupported() bool { return false }

// hubReexec is unreachable on this platform: RestartHub refuses before the
// exec step. It returns an error instead of panicking so a future caller
// cannot accidentally take the process down with it.
func hubReexec(string, []string, []string) error {
	return errors.New("此平台不支持原地重启中心")
}

// hubEnviron is the environment inherited by the replacement image.
func hubEnviron() []string { return os.Environ() }
