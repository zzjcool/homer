//go:build !linux

package agentd

import (
	"runtime"

	"github.com/zzjcool/homer/internal/hub"
)

func collectPlatform() *hub.HostSnapshot {
	return &hub.HostSnapshot{OS: runtime.GOOS, Arch: runtime.GOARCH}
}

func cpuModel() string { return "" }

func readCPUSample() (cpuTimes, int, bool) { return cpuTimes{}, 0, false }
