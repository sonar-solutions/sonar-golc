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
		"populated": {Platform: "azure", Scanned: 22, Analyzed: 10, Archived: 2, Empty: 3, Excluded: 5, Skipped: 2},
	}
	for name, summary := range cases {
		t.Run(name, func(t *testing.T) {
			pdf := gofpdf.New("P", "mm", "A4", "")
			pdf.SetFont("Helvetica", "", 8)
			pdf.AddPage()
			renderScanSummarySection(pdf, summary, 4, 3, 15.0, 180.0)
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
		Skipped:  1,
	}
	if err := SaveScanSummary(base, in); err != nil {
		t.Fatalf("SaveScanSummary: %v", err)
	}

	got := LoadScanSummary(base)
	if got == nil {
		t.Fatal("LoadScanSummary returned nil for a summary that was just saved")
	}
	if want := 21; got.Scanned != want {
		t.Errorf("Scanned = %d, want %d (Analyzed+Archived+Empty+Excluded+Skipped)", got.Scanned, want)
	}
	if got.Platform != "azure" || got.Analyzed != 10 || got.Archived != 2 || got.Empty != 3 || got.Excluded != 5 || got.Skipped != 1 {
		t.Errorf("round-trip mismatch: %+v", *got)
	}
}

func TestNewScanSummary(t *testing.T) {
	s := NewScanSummary("gitlab", 10, 2, 3, 4, 1)
	if s.Platform != "gitlab" || s.Analyzed != 10 || s.Archived != 2 || s.Empty != 3 || s.Excluded != 4 || s.Skipped != 1 {
		t.Errorf("unexpected mapping: %+v", s)
	}
	// Scanned is left for SaveScanSummary to derive, not set by the constructor.
	if s.Scanned != 0 {
		t.Errorf("Scanned should be 0 until persisted, got %d", s.Scanned)
	}
}

func TestPersistScanSummary(t *testing.T) {
	base := t.TempDir()
	PersistScanSummary(base, ScanSummary{Platform: "gitlab", Analyzed: 4, Archived: 1})

	got := LoadScanSummary(base)
	if got == nil {
		t.Fatal("PersistScanSummary did not write a loadable summary")
	}
	if got.Scanned != 5 || got.Analyzed != 4 || got.Archived != 1 {
		t.Errorf("unexpected persisted summary: %+v", *got)
	}
}

func TestSaveScanSummaryErrorPath(t *testing.T) {
	// Use a regular file as the base dir so MkdirAll(<file>/config) fails,
	// exercising SaveScanSummary's error return.
	base := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(base, []byte("x"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := SaveScanSummary(base, ScanSummary{Platform: "x"}); err == nil {
		t.Error("expected an error when the base path is a file, got nil")
	}
	// PersistScanSummary must swallow (log) the error, not panic or propagate.
	PersistScanSummary(base, ScanSummary{Platform: "x"})
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
