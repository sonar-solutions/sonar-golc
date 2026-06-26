package gogit

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// makeLocalRepo creates a throwaway local git repository with one commit on
// branch "master", so clones can be exercised fully offline.
func makeLocalRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "master")
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// TestGetreposLocalClone clones a local repo into a configured work dir and
// verifies the clone lands there.
func TestGetreposLocalClone(t *testing.T) {
	repo := makeLocalRepo(t)
	work := t.TempDir()

	dst, err := Getrepos(repo, "master", "", work)
	if err != nil {
		t.Fatalf("Getrepos: %v", err)
	}
	if filepath.Dir(dst) != work {
		t.Errorf("clone landed in %q, want under %q", dst, work)
	}
	if _, err := os.Stat(filepath.Join(dst, "main.go")); err != nil {
		t.Errorf("expected cloned main.go, stat err=%v", err)
	}
}

// TestGetreposBadWorkDir verifies an unusable work dir is reported as an error
// (clear failure rather than a panic deep in the clone).
func TestGetreposBadWorkDir(t *testing.T) {
	// A file used as the work-dir parent makes MkdirAll fail.
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := Getrepos("https://example.invalid/x.git", "master", "", filepath.Join(f, "sub")); err == nil {
		t.Error("expected error for unusable work dir, got nil")
	}
}
