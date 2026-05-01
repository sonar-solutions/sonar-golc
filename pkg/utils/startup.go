package utils

import (
	"os"
	"path/filepath"
)

// ChdirToBinaryDir changes the working directory to the folder containing the
// running executable. This ensures all relative paths (Results/, Logs/,
// config.json, imgs/) resolve to the binary's own directory regardless of how
// it was launched (double-click, terminal, PATH). Subprocesses inherit the
// corrected CWD automatically.
func ChdirToBinaryDir() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	_ = os.Chdir(filepath.Dir(exePath))
}
