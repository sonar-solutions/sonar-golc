package utils

import (
	"fmt"
	"os"
	"sync"
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

// Temp-clone registry.
//
// Deferred cleanup in the analysis code removes a temp clone on every normal
// return, but golc also calls os.Exit() on a number of error paths (e.g. an
// analysis that produced 0 lines of code), and os.Exit() does NOT run deferred
// functions. To keep those paths from leaking clones (issue #81), every temp
// clone is registered here when created and the process sweeps the registry
// just before calling os.Exit().
var (
	tempCloneMu sync.Mutex
	tempClones  = map[string]struct{}{}
)

// RegisterTempClone records a temp clone/extraction directory so it can be
// swept before an os.Exit(). No-op for empty paths.
func RegisterTempClone(path string) {
	if path == "" {
		return
	}
	tempCloneMu.Lock()
	tempClones[path] = struct{}{}
	tempCloneMu.Unlock()
}

// UnregisterTempClone drops a path from the registry (e.g. once it has already
// been removed by deferred cleanup on the normal path).
func UnregisterTempClone(path string) {
	tempCloneMu.Lock()
	delete(tempClones, path)
	tempCloneMu.Unlock()
}

// CleanupTempClones removes any still-registered temp clones. It is safe to
// call multiple times and concurrently; RemoveAll on an already-deleted path
// is a no-op. Call this immediately before os.Exit().
func CleanupTempClones() {
	tempCloneMu.Lock()
	paths := make([]string, 0, len(tempClones))
	for p := range tempClones {
		paths = append(paths, p)
	}
	tempClones = map[string]struct{}{}
	tempCloneMu.Unlock()

	for _, p := range paths {
		_ = os.RemoveAll(p)
	}
}
