package utils

import (
	"os"
	"path/filepath"
)

// GetLogoPath returns the path to imgs/Logob.png, resolved relative to the
// running executable so the binary finds the asset regardless of the caller's
// working directory. Falls back to the CWD-relative path when the executable
// path cannot be determined.
func GetLogoPath() string {
	const rel = "imgs/Logob.png"
	if exePath, err := os.Executable(); err == nil {
		candidate := filepath.Join(filepath.Dir(exePath), rel)
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return rel
}
