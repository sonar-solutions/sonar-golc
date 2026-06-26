package utils

import (
	"fmt"
	"os"
)

// WorkDirEnvVar is the name of the environment variable that provides a global
// override for the directory where repositories are cloned/extracted before analysis.
const WorkDirEnvVar = "GOLC_WORKDIR"

// ResolveWorkDir returns the base directory under which temporary clones/extractions
// should be created, applying the following precedence:
//
//  1. configured  - the per-platform "WorkDir" value from config.json / the web UI
//  2. GOLC_WORKDIR - a global environment-variable override
//  3. os.TempDir() - the historical default (unchanged behavior)
//
// The chosen directory is created if it does not exist and is checked for
// writability, so callers fail fast with a clear error instead of a cryptic
// "no space left on device" / "no such file or directory" deep inside a clone.
func ResolveWorkDir(configured string) (string, error) {
	dir := configured
	if dir == "" {
		dir = os.Getenv(WorkDirEnvVar)
	}
	if dir == "" {
		dir = os.TempDir()
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("work directory %q cannot be created: %w", dir, err)
	}

	// Verify writability up front: create and remove a probe file.
	probe, err := os.CreateTemp(dir, "gcloc-workdir-probe-*")
	if err != nil {
		return "", fmt.Errorf("work directory %q is not writable: %w", dir, err)
	}
	probeName := probe.Name()
	probe.Close()
	os.Remove(probeName)

	return dir, nil
}
