package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// TestRenderScanSummarySection exercises the PDF section renderer for the nil
// (older result set), zero-count, and populated cases — it must not panic.
func TestRenderScanSummarySection(t *testing.T) {
	cases := map[string]*ScanSummary{
		"nil":       nil,
		"zero":      {Platform: "azure"},
		"populated": {Platform: "azure", Scanned: 20, Analyzed: 10, Archived: 2, Empty: 3, Excluded: 5},
	}
	for name, summary := range cases {
		t.Run(name, func(t *testing.T) {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.SetFont("Helvetica", "", 8)
			pdf.AddPage()
			renderScanSummarySection(pdf, summary, 4, 15.0, 180.0)
			if err := pdf.OutputFileAndClose(filepath.Join(t.TempDir(), "out.pdf")); err != nil {
				t.Fatalf("render/output failed: %v", err)
			}
		})
	}
}

func TestSaveAndLoadScanSummary(t *testing.T) {
	base := t.TempDir()

	// Scanned is intentionally left 0 to prove SaveScanSummary derives it from
	// the category counts.
	in := ScanSummary{
		Platform: "azure",
		Analyzed: 10,
		Archived: 2,
		Empty:    3,
		Excluded: 5,
	}
	if err := SaveScanSummary(base, in); err != nil {
		t.Fatalf("SaveScanSummary: %v", err)
	}

	got := LoadScanSummary(base)
	if got == nil {
		t.Fatal("LoadScanSummary returned nil for a summary that was just saved")
	}
	if want := 20; got.Scanned != want {
		t.Errorf("Scanned = %d, want %d (Analyzed+Archived+Empty+Excluded)", got.Scanned, want)
	}
	if got.Platform != "azure" || got.Analyzed != 10 || got.Archived != 2 || got.Empty != 3 || got.Excluded != 5 {
		t.Errorf("round-trip mismatch: %+v", *got)
	}
}

func TestLoadScanSummaryMissingFile(t *testing.T) {
	if got := LoadScanSummary(t.TempDir()); got != nil {
		t.Errorf("LoadScanSummary on missing file = %+v, want nil", got)
	}
}

func TestLoadScanSummaryInvalidJSON(t *testing.T) {
	base := t.TempDir()
	path := ScanSummaryPath(base)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte("{not json"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if got := LoadScanSummary(base); got != nil {
		t.Errorf("LoadScanSummary on invalid JSON = %+v, want nil", got)
	}
}
