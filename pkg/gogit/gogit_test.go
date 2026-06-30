package gogit

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	dst, err := Getrepos(repo, "master", "", work, 30*time.Second)
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
	if _, err := Getrepos("https://example.invalid/x.git", "master", "", filepath.Join(f, "sub"), 30*time.Second); err == nil {
		t.Error("expected error for unusable work dir, got nil")
	}
}

// TestGetreposCloneTimeout verifies that a clone which cannot complete in time
// returns a timeout error (rather than hanging) and leaves no temp clone behind.
// It points at a non-routable address so the connection stalls until the deadline.
func TestGetreposCloneTimeout(t *testing.T) {
	work := t.TempDir()

	// 198.51.100.0/24 (TEST-NET-2) is reserved and non-routable, so the TCP
	// connect blocks until our context deadline fires.
	_, err := Getrepos("https://198.51.100.1/some/repo.git", "master", "", work, 1*time.Second)
	if err == nil {
		t.Fatal("expected a timeout/clone error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") && !strings.Contains(err.Error(), "clone failed") {
		t.Errorf("expected a timeout or clone-failed error, got: %v", err)
	}

	// The partial/failed clone must not be left behind in the work dir.
	entries, rerr := os.ReadDir(work)
	if rerr != nil {
		t.Fatalf("read work dir: %v", rerr)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "gcloc-extract-") {
			t.Errorf("failed clone left temp dir behind: %s", e.Name())
		}
	}
}
