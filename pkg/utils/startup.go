package utils

import (
	"os"
	"path/filepath"
)

// DataDirEnvVar names the environment variable that overrides the folder GoLC
// runs from. The container image sets it so config.json, Results/ and Logs/
// land on the mounted volume instead of next to the read-only binaries.
const DataDirEnvVar = "GOLC_DATA_DIR"

// ChdirToBinaryDir changes the working directory to the folder containing the
// running executable. This ensures all relative paths (Results/, Logs/,
// config.json, imgs/) resolve to the binary's own directory regardless of how
// it was launched (double-click, terminal, PATH). Subprocesses inherit the
// corrected CWD automatically. GOLC_DATA_DIR, when set, is used instead.
func ChdirToBinaryDir() {
	if dir := os.Getenv(DataDirEnvVar); dir != "" {
		_ = os.Chdir(dir)
		return
	}
	exePath, err := os.Executable()
	if err != nil {
		return
	}
	if resolved, err := filepath.EvalSymlinks(exePath); err == nil {
		exePath = resolved
	}
	_ = os.Chdir(filepath.Dir(exePath))
}
