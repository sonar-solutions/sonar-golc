package getter

import (
	"os"
	"path/filepath"
	"testing"
)

// TestGetterLocalDir copies a local directory into a configured work dir
// (go-getter handles local paths offline) and verifies the destination.
func TestGetterLocalDir(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	work := t.TempDir()

	// go-getter may symlink a local source dir (returning the origin path) or
	// copy it under the work dir; either way the new ResolveWorkDir/register
	// lines run. Assert only that it succeeded and the content is reachable.
	dst, err := Getter(src, work)
	if err != nil {
		t.Fatalf("Getter: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dst, "main.go")); err != nil {
		t.Errorf("expected extracted main.go at %q, stat err=%v", dst, err)
	}
}

// TestGetterBadWorkDir verifies an unusable work dir is reported as an error.
func TestGetterBadWorkDir(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := Getter(t.TempDir(), filepath.Join(f, "sub")); err == nil {
		t.Error("expected error for unusable work dir, got nil")
	}
}
