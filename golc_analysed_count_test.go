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

// A skipped repository reports on the channel like any other - the skip path sends before it
// returns. Counting messages therefore counts skips as analysed, which is how a run that
// skipped two of thirty still announced "30 analyzed" while printing a two-repository skip
// warning directly above it. The lines of a skipped repository are absent from the totals just
// as surely as those of one that vanished, so it must not appear in the headline.
func TestAnalyseReposListExcludesSkippedRepositories(t *testing.T) {
	out := captureLog(t)

	// Two of five fail: they report, but as skips.
	analyse := func(project interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		if project.(int) < 2 {
			results <- repoSkipped
			return
		}
		results <- repoAnalysed
	}

	got := AnalyseReposList(t.TempDir(), testPlatformConfig(true), testRepos(5), analyse)

	if got != 3 {
		t.Errorf("count = %d, want 3: two repositories were skipped, so their lines are not in "+
			"the totals and they cannot be part of the analyzed headline", got)
	}
	if !strings.Contains(out.String(), "5 repositories found: 3 analyzed, 2 skipped") {
		t.Errorf("the shortfall was not accounted for in the log; got:\n%s", out.String())
	}
}

// A skip is a reported outcome, not a disappearance. Raising the "did not complete ... with no
// error of their own" alarm for it would be false: the repository did fail, visibly, and is
// already in the skip report. Crying wolf here devalues the alarm for the case that has no
// other signal at all.
func TestAnalyseReposListDoesNotCallASkipAVanishedRepository(t *testing.T) {
	out := captureLog(t)

	analyse := func(project interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		if project.(int) == 0 {
			results <- repoSkipped
			return
		}
		results <- repoAnalysed
	}

	AnalyseReposList(t.TempDir(), testPlatformConfig(true), testRepos(3), analyse)

	if strings.Contains(out.String(), "did not complete") {
		t.Errorf("a skipped repository was reported as having vanished:\n%s", out.String())
	}
}

// The two failure modes are independent and can happen in the same run, so the report has to
// keep them apart: one repository skipped and one gone must not be summed into "two of
// something" - they need different actions from whoever reads it.
func TestAnalyseReposListSeparatesSkippedFromVanished(t *testing.T) {
	out := captureLog(t)

	analyse := func(project interface{}, _ string, _ map[string]interface{},
		_ *spinner.Spinner, results chan int, _ *atomic.Int64) {
		switch project.(int) {
		case 0:
			results <- repoSkipped
		case 1:
			return // vanishes: no report at all
		default:
			results <- repoAnalysed
		}
	}

	got := AnalyseReposList(t.TempDir(), testPlatformConfig(true), testRepos(4), analyse)

	if got != 2 {
		t.Errorf("count = %d, want 2 (4 found, 1 skipped, 1 vanished)", got)
	}

	logged := out.String()

	// One of each. Summing them into "2 skipped" would say the repository that reported
	// nothing had reported a failure, which is exactly the conflation this test exists to
	// prevent - and it would contradict the "did not complete" line printed just below it.
	if !strings.Contains(logged, "4 repositories found: 2 analyzed, 1 skipped, 1 did not report") {
		t.Errorf("the two shortfalls were not named separately; got:\n%s", logged)
	}
	if strings.Contains(logged, "2 skipped") {
		t.Errorf("the vanished repository was counted as skipped; got:\n%s", logged)
	}
	if !strings.Contains(logged, "1 of 4 repositories did not complete") {
		t.Errorf("the vanished repository was not called out separately; got:\n%s", logged)
	}
}
