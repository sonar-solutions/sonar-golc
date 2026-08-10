package getazure

import (
	"bytes"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

// captureSummaryLog redirects the shared logger for one test and restores it afterwards.
func captureSummaryLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	logger := utils.SharedLogger()
	previous := logger.Out

	var out bytes.Buffer
	logger.SetOutput(&out)
	t.Cleanup(func() { logger.SetOutput(previous) })

	return &out
}

// A repository can be discovered and then dropped before analysis for having no branch
// matching the configured one. It lands in none of the empty/excluded/archived counters, so
// nothing in the summary accounts for it and its lines are simply missing from the total.
//
// An organisation that pins "main" across a mix of "main" and "master" repositories hits this
// on every scan, and silent omission is the worst failure mode for a licence-sizing tool.
func TestPrintSummaryReportsRepositoriesDroppedBeforeAnalysis(t *testing.T) {
	out := captureSummaryLog(t)

	printSummary("DefaultCollection", SummaryStats{
		NbRepos:          3,
		Analyzed:         2,
		TotalBranches:    2,
		DiscoverySkipped: 1,
	})

	logged := out.String()
	if !strings.Contains(logged, "1 of 3 discovered repository(ies) will not be analyzed") {
		t.Errorf("the shortfall was never reported, so a repository missing from the totals "+
			"leaves no trace at all.\ngot:\n%s", logged)
	}
}

// The headline has to come from the number of repositories that will actually be analysed,
// not from discovered-minus-filtered. Those two agree right up until a repository is dropped
// for its branch, and then the arithmetic overstates the scope of the scan.
func TestPrintSummaryReportsAnalysedCountNotTheArithmetic(t *testing.T) {
	out := captureSummaryLog(t)

	// Three discovered, none in a filtered category, but only two carry the configured
	// branch: NbRepos - empty - excluded - archived would say three.
	printSummary("DefaultCollection", SummaryStats{
		NbRepos:          3,
		Analyzed:         2,
		TotalBranches:    2,
		DiscoverySkipped: 1,
	})

	logged := out.String()
	if !strings.Contains(logged, "Total Repositories that will be analyzed: 2") {
		t.Errorf("expected the analysed count (2).\ngot:\n%s", logged)
	}
	if strings.Contains(logged, "Total Repositories that will be analyzed: 3") {
		t.Errorf("reported 3 repositories as about to be analysed when only 2 were.\ngot:\n%s", logged)
	}
}

// The filtered categories must keep reconciling as they did before: a run where every
// non-filtered repository is analysed has no shortfall and must not claim one.
func TestPrintSummaryReconcilesTheFilteredCategories(t *testing.T) {
	out := captureSummaryLog(t)

	printSummary("DefaultCollection", SummaryStats{
		NbRepos:       30,
		EmptyRepo:     2,
		TotalExclude:  2,
		TotalArchiv:   1,
		Analyzed:      25, // 30 - 2 - 2 - 1
		TotalBranches: 25,
	})

	logged := out.String()
	if !strings.Contains(logged, "Total Repositories that will be analyzed: 25") {
		t.Errorf("the filtered-category arithmetic no longer reconciles.\ngot:\n%s", logged)
	}
	if strings.Contains(logged, "will not be analyzed") {
		t.Errorf("warned about a shortfall when every unfiltered repository was analysed.\ngot:\n%s", logged)
	}
}

// The warning is conditional, so a clean run stays quiet. A summary that cried shortfall on
// every scan would train operators to ignore the one that matters.
func TestPrintSummarySaysNothingWhenNothingWasDropped(t *testing.T) {
	out := captureSummaryLog(t)

	printSummary("DefaultCollection", SummaryStats{
		NbRepos:       30,
		Analyzed:      30,
		TotalBranches: 30,
	})

	logged := out.String()
	if strings.Contains(logged, "will not be analyzed") {
		t.Errorf("warned about a shortfall on a complete run.\ngot:\n%s", logged)
	}
	if !strings.Contains(logged, "Total Repositories that will be analyzed: 30") {
		t.Errorf("expected all 30 reported as analysed.\ngot:\n%s", logged)
	}
}
