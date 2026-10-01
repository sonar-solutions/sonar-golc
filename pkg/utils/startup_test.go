package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChdirToBinaryDir(t *testing.T) {
	t.Run("data dir env var wins over the binary's folder", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(t.TempDir())
		t.Setenv(DataDirEnvVar, dir)
		ChdirToBinaryDir()
		assertCwd(t, dir)
	})

	t.Run("binary's folder when env var is empty", func(t *testing.T) {
		t.Chdir(t.TempDir())
		t.Setenv(DataDirEnvVar, "")
		exe, err := os.Executable()
		if err != nil {
			t.Fatalf("os.Executable: %v", err)
		}
		ChdirToBinaryDir()
		assertCwd(t, filepath.Dir(exe))
	})
}

func assertCwd(t *testing.T, want string) {
	t.Helper()
	got, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	// TempDir paths go through symlinks on macOS (/var -> /private/var).
	wantResolved, _ := filepath.EvalSymlinks(want)
	gotResolved, _ := filepath.EvalSymlinks(got)
	if gotResolved != wantResolved {
		t.Errorf("cwd = %q, want %q", gotResolved, wantResolved)
	}
}
