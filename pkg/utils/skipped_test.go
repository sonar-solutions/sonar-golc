package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// TestRenderSkippedReposSection exercises the PDF section renderer for both the
// empty and populated cases, including a list long enough to trigger a page break
// and a very long reason that must be truncated — it must not panic.
func TestRenderSkippedReposSection(t *testing.T) {
	cases := map[string][]SkippedRepo{
		"empty": nil,
		"one":   {{ProjectKey: "org", RepoSlug: "repo", Branch: "main", Reason: "clone timed out after 15m0s"}},
	}
	// A long list with a long reason to cover truncation + page-break paths.
	var many []SkippedRepo
	for i := 0; i < 60; i++ {
		many = append(many, SkippedRepo{
			RepoSlug: "some-really-quite-long-repository-name-number",
			Branch:   "feature/a-fairly-long-branch-name",
			Reason:   "clone failed: error: RPC failed; curl 56 recv failure: connection reset by peer the remote end hung up unexpectedly",
		})
	}
	cases["many"] = many

	for name, repos := range cases {
		t.Run(name, func(t *testing.T) {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.SetFont("Helvetica", "", 8)
			pdf.AddPage()
			renderSkippedReposSection(pdf, repos, 15.0, 180.0)
			if err := pdf.OutputFileAndClose(filepath.Join(t.TempDir(), "out.pdf")); err != nil {
				t.Fatalf("render/output failed: %v", err)
			}
		})
	}
}

// TestSkippedReposRoundTrip verifies that the skipped-repos list persists and
// reloads intact from <base>/config/analysis_skipped.json.
func TestSkippedReposRoundTrip(t *testing.T) {
	base := t.TempDir()

	want := []SkippedRepo{
		{ProjectKey: "acme", RepoSlug: "bi_publishing", Branch: "main", Reason: "clone timed out after 15m0s"},
		{ProjectKey: "", RepoSlug: "lonely-repo", Branch: "develop", Reason: "analysis error: boom"},
	}

	if err := SaveSkippedRepos(base, want); err != nil {
		t.Fatalf("SaveSkippedRepos: %v", err)
	}

	// File must land at the canonical path.
	if _, err := os.Stat(filepath.Join(base, "config", "analysis_skipped.json")); err != nil {
		t.Fatalf("expected skipped file at canonical path: %v", err)
	}

	got := LoadSkippedRepos(base)
	if len(got) != len(want) {
		t.Fatalf("loaded %d entries, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestSaveSkippedReposEmptyClearsStale verifies that saving an empty list writes a
// valid (empty) file so a clean re-run clears entries from a previous analysis.
func TestSaveSkippedReposEmptyClearsStale(t *testing.T) {
	base := t.TempDir()

	if err := SaveSkippedRepos(base, []SkippedRepo{{RepoSlug: "x", Branch: "main", Reason: "stale"}}); err != nil {
		t.Fatalf("seed save: %v", err)
	}
	if err := SaveSkippedRepos(base, nil); err != nil {
		t.Fatalf("empty save: %v", err)
	}
	if got := LoadSkippedRepos(base); len(got) != 0 {
		t.Errorf("after empty save, loaded %d entries, want 0", len(got))
	}
}

// TestLoadSkippedReposMissing verifies a missing file yields an empty slice (not an
// error) so reports render fine on result sets that predate this feature.
func TestLoadSkippedReposMissing(t *testing.T) {
	if got := LoadSkippedRepos(t.TempDir()); len(got) != 0 {
		t.Errorf("missing file: got %d entries, want 0", len(got))
	}
}
