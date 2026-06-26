package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveWorkDir(t *testing.T) {
	t.Run("empty falls back to os.TempDir", func(t *testing.T) {
		t.Setenv(WorkDirEnvVar, "")
		got, err := ResolveWorkDir("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != os.TempDir() {
			t.Errorf("expected %q, got %q", os.TempDir(), got)
		}
	})

	t.Run("env var override used when configured is empty", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv(WorkDirEnvVar, dir)
		got, err := ResolveWorkDir("")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dir {
			t.Errorf("expected %q, got %q", dir, got)
		}
	})

	t.Run("configured value takes precedence over env", func(t *testing.T) {
		envDir := t.TempDir()
		cfgDir := t.TempDir()
		t.Setenv(WorkDirEnvVar, envDir)
		got, err := ResolveWorkDir(cfgDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != cfgDir {
			t.Errorf("expected configured %q to win, got %q", cfgDir, got)
		}
	})

	t.Run("creates a missing directory", func(t *testing.T) {
		t.Setenv(WorkDirEnvVar, "")
		dir := filepath.Join(t.TempDir(), "nested", "golc-tmp")
		got, err := ResolveWorkDir(dir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != dir {
			t.Errorf("expected %q, got %q", dir, got)
		}
		if info, statErr := os.Stat(dir); statErr != nil || !info.IsDir() {
			t.Errorf("expected directory %q to be created", dir)
		}
	})

	t.Run("errors when path is not writable", func(t *testing.T) {
		t.Setenv(WorkDirEnvVar, "")
		// A regular file cannot be used as a directory.
		file := filepath.Join(t.TempDir(), "afile")
		if err := os.WriteFile(file, []byte("x"), 0644); err != nil {
			t.Fatalf("setup failed: %v", err)
		}
		if _, err := ResolveWorkDir(file); err == nil {
			t.Errorf("expected error for non-directory path %q, got nil", file)
		}
	})
}
