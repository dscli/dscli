package processutil

import (
	"fmt"
	"os"
	"path/filepath"
)

// IsProcessRunning checks whether a dscli chat process is currently holding
// the project-level lockfile at <project>/.dscli/locks/dscli.lock.
func IsProcessRunning(projectPath string) bool {
	lockPath := filepath.Join(projectPath, ".dscli", "locks", "dscli.lock")
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return false
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil || pid == 0 {
		return false
	}

	// Check if process is alive.
	return IsAlive(pid)
}
