//go:build golc
// +build golc

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/briandowns/spinner"
)

// makeLocalRepo creates a throwaway local git repo (one commit on "master")
// so the clone path can be exercised offline.
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
		[]byte("package main\n\nfunc main() { println(\"x\") }\n"), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	run("add", ".")
	run("commit", "-m", "init")
	return dir
}

// TestPerformRepoAnalysisCloneCleanup drives performRepoAnalysis through the
// disposable-clone path (MainBranch set => gogit clone into WorkDir) and
// verifies the clone is removed afterwards (Fix B) and the work dir is left
// empty.
func TestPerformRepoAnalysisCloneCleanup(t *testing.T) {
	repo := makeLocalRepo(t)
	dest := t.TempDir()
	work := t.TempDir()

	params := RepoParams{
		ProjectKey: "org",
		RepoSlug:   "repo",
		MainBranch: "master",
		PathToScan: repo, // local path => offline clone
		WorkDir:    work,
	}
	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	results := make(chan int, 1)
	count := 0

	performRepoAnalysis(params, dest, spin, results, &count, nil, nil, nil, nil, false, false)
	<-results

	entries, _ := os.ReadDir(work)
	if len(entries) != 0 {
		t.Errorf("temp clone not cleaned up; work dir has %d entries", len(entries))
	}
}

// TestGetWorkDir covers the optional per-platform WorkDir config accessor.
func TestGetWorkDir(t *testing.T) {
	cases := []struct {
		name string
		cfg  map[string]interface{}
		want string
	}{
		{"set", map[string]interface{}{"WorkDir": "/data/tmp"}, "/data/tmp"},
		{"empty", map[string]interface{}{"WorkDir": ""}, ""},
		{"absent", map[string]interface{}{}, ""},
		{"nil value", map[string]interface{}{"WorkDir": nil}, ""},
		{"wrong type", map[string]interface{}{"WorkDir": 123}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := getWorkDir(c.cfg); got != c.want {
				t.Errorf("getWorkDir(%v) = %q, want %q", c.cfg, got, c.want)
			}
		})
	}
}

// writeSourceDir creates a temp directory containing one recognised source file.
func writeSourceDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {\n\tprintln(\"hi\")\n}\n"), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	return dir
}

// TestPerformRepoAnalysisLocalDir drives performRepoAnalysis against a local
// directory (MainBranch empty => no clone, no network). This exercises the
// WorkDir plumbing and the deferred-cleanup path on a non-disposable repo
// (the clone is the user's dir, so it must be kept).
func TestPerformRepoAnalysisLocalDir(t *testing.T) {
	src := writeSourceDir(t)
	dest := t.TempDir()

	params := RepoParams{
		ProjectKey: "org",
		RepoSlug:   "repo",
		MainBranch: "", // empty => analyse the local path directly
		PathToScan: src,
		WorkDir:    "",
	}
	spin := spinner.New(spinner.CharSets[35], 100*time.Millisecond)
	results := make(chan int, 1)
	count := 0

	performRepoAnalysis(params, dest, spin, results, &count, nil, nil, nil, nil, false, false)

	if got := <-results; got != 1 {
		t.Errorf("expected 1 result signal, got %d", got)
	}
	// Non-disposable source dir must NOT be deleted by the cleanup defer.
	if _, err := os.Stat(src); err != nil {
		t.Errorf("source dir should be preserved, stat err=%v", err)
	}
}

// TestAnalyseDirectoryLocal drives the File-platform directory path.
func TestAnalyseDirectoryLocal(t *testing.T) {
	src := writeSourceDir(t)
	dest := t.TempDir()
	count := 1

	// Should analyse the directory in place without panicking or deleting it.
	analyseDirectory(src, false, false, nil, nil, nil, nil, dest, &count)

	if _, err := os.Stat(src); err != nil {
		t.Errorf("source dir should be preserved, stat err=%v", err)
	}
}

// TestAnalyseReposListSingleThreaded covers the non-multithreading dispatch
// branch (the one that previously deadlocked) using a stub worker, so no real
// repository analysis/network is needed.
func TestAnalyseReposListSingleThreaded(t *testing.T) {
	dest := t.TempDir()
	cfg := map[string]interface{}{
		"Multithreading":    false,
		"NumberWorkerRepos": float64(5),
		"Workers":           float64(2),
	}
	repolist := []interface{}{"repo-a", "repo-b"}
	var processed int

	stub := func(_ interface{}, _ string, _ map[string]interface{}, _ *spinner.Spinner, results chan int, count *int) {
		processed++
		results <- 1
	}

	done := make(chan int, 1)
	go func() {
		done <- AnalyseReposList(dest, cfg, repolist, stub)
	}()

	select {
	case n := <-done:
		if n != len(repolist) {
			t.Errorf("AnalyseReposList returned %d, want %d", n, len(repolist))
		}
	case <-time.After(5 * time.Second):
		t.Fatal("AnalyseReposList deadlocked in single-threaded mode")
	}
	if processed != len(repolist) {
		t.Errorf("stub processed %d repos, want %d", processed, len(repolist))
	}
}
