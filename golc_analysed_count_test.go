//go:build golc
// +build golc

package main

import (
	"bytes"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/briandowns/spinner"
)

// testPlatformConfig is the minimum AnalyseReposList reads: the three concurrency keys.
func testPlatformConfig(multithreading bool) map[string]interface{} {
	return map[string]interface{}{
		"Multithreading":    multithreading,
		"NumberWorkerRepos": float64(4),
		"Workers":           float64(2),
	}
}

// captureLog redirects the package logger for one test and restores it afterwards.
// Restoring to nil instead would leave the next test writing to a nil writer, which panics
// inside logrus - a failure that shows up in an unrelated test and points nowhere useful.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var out bytes.Buffer
	previous := logger.Out
	logger.SetOutput(&out)
	t.Cleanup(func() { logger.SetOutput(previous) })

	return &out
}

func testRepos(n int) []interface{} {
	repos := make([]interface{}, n)
	for i := range repos {
		repos[i] = i
	}

	return repos
}

// The count AnalyseReposList returns is displayed as the number of repositories *analyzed*.
// Returning the number *found* instead makes a repository that vanished mid-run indissoluble
// from one that was counted, so the totals under-report with nothing to say they did. In a
// tool used to size a licence that is the worst way to be wrong: quietly.
func TestAnalyseReposListCountsAnalysedNotFound(t *testing.T) {
	var reported atomic.Int64

	// Repo 2 returns without reporting - the disappearance being guarded against.
	analyse := func(project interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		if project.(int) == 2 {
			return
		}
		reported.Add(1)
		results <- 1
	}

	got := AnalyseReposList(t.TempDir(), testPlatformConfig(true), testRepos(5), analyse)

	if got != 4 {
		t.Errorf("count = %d, want 4: one of the five repositories never completed, so "+
			"reporting %d would count a repository whose lines are absent from the totals",
			got, got)
	}
	if reported.Load() != 4 {
		t.Fatalf("the fake reported %d times, so the test is not exercising what it claims",
			reported.Load())
	}
}

// The mismatch has to be said out loud. A number that is quietly lower than expected reads as
// "that is how big the estimate is", not as "part of the estimate is missing".
func TestAnalyseReposListLogsWhenRepositoriesGoMissing(t *testing.T) {
	out := captureLog(t)

	analyse := func(project interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		if project.(int) == 0 {
			return
		}
		results <- 1
	}

	AnalyseReposList(t.TempDir(), testPlatformConfig(true), testRepos(3), analyse)

	logged := out.String()
	if !strings.Contains(logged, "1 of 3 repositories did not complete") {
		t.Errorf("missing repositories were not reported; log was:\n%s", logged)
	}
	if !strings.Contains(logged, "under-count") {
		t.Errorf("the log does not say the totals are wrong, only that something happened; "+
			"log was:\n%s", logged)
	}
}

// The complement, and the case that runs every day: when nothing goes missing there must be no
// warning at all. A tool that cries wolf on healthy scans gets its warnings tuned out.
func TestAnalyseReposListIsSilentWhenEveryRepositoryCompletes(t *testing.T) {
	out := captureLog(t)

	analyse := func(_ interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		results <- 1
	}

	if got := AnalyseReposList(t.TempDir(), testPlatformConfig(true), testRepos(3), analyse); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}
	if strings.Contains(out.String(), "did not complete") {
		t.Errorf("a healthy scan warned about missing repositories:\n%s", out.String())
	}
}

// Sequential mode takes the same path with concurrency pinned to 1, and the drain must not
// deadlock there either - the channel is buffered to the repository count for that reason.
func TestAnalyseReposListCountsWithMultithreadingOff(t *testing.T) {
	analyse := func(_ interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		results <- 1
	}

	if got := AnalyseReposList(t.TempDir(), testPlatformConfig(false), testRepos(3), analyse); got != 3 {
		t.Errorf("count = %d, want 3", got)
	}
}
