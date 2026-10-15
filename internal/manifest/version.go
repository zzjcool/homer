package manifest

import (
	"strings"
)

// VersionedListCommand asks a tool to print versions when its stock list
// command omits them. pi list has no such flag; bare npm:name ids stay bare
// so floating/latest installs are not pinned for remote machines.
func VersionedListCommand(command string) string {
	if strings.TrimSpace(command) == "code --list-extensions" {
		return "code --list-extensions --show-versions"
	}
	return command
}
