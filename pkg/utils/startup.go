package utils

import (
	"os"
	"path/filepath"
	"strconv"
)

// ResultsPublicPortEnvVar names the environment variable that tells the setup
// UI which port the browser reaches the results dashboard on. In a container
// the dashboard binds 8090, but Docker can publish it on another host port
// (-p 9090:8090), and nothing inside the container can see that mapping.
const ResultsPublicPortEnvVar = "GOLC_RESULTS_PUBLIC_PORT"

// ResultsPublicPort returns the port to send the browser to for the results
// dashboard: GOLC_RESULTS_PUBLIC_PORT when it holds a valid port, otherwise
// boundPort, the port the dashboard actually listens on.
func ResultsPublicPort(boundPort int) int {
	if p, err := strconv.Atoi(os.Getenv(ResultsPublicPortEnvVar)); err == nil && p > 0 && p < 65536 {
		return p
	}
	return boundPort
}

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
