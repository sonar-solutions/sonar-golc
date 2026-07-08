//go:build resultsall
// +build resultsall

package main

import (
	"bytes"
	"html/template"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
)

func TestBuildScanSummaryView(t *testing.T) {
	if got := buildScanSummaryView(nil, 3); got != nil {
		t.Errorf("nil summary should produce nil view, got %+v", got)
	}

	// Analyzed subtracts the repos that failed during analysis (Skipped).
	v := buildScanSummaryView(&utils.ScanSummary{
		Scanned: 20, Analyzed: 10, Archived: 2, Empty: 3, Excluded: 5,
	}, 4)
	if v == nil {
		t.Fatal("expected non-nil view")
	}
	if v.Analyzed != 6 || v.Skipped != 4 {
		t.Errorf("Analyzed=%d Skipped=%d, want 6 and 4", v.Analyzed, v.Skipped)
	}
	if v.Scanned != 20 || v.Archived != 2 || v.Empty != 3 || v.Excluded != 5 {
		t.Errorf("unexpected passthrough values: %+v", *v)
	}

	// Analyzed must floor at 0 when more repos were skipped than projected.
	if v2 := buildScanSummaryView(&utils.ScanSummary{Analyzed: 2}, 5); v2.Analyzed != 0 {
		t.Errorf("Analyzed should floor at 0, got %d", v2.Analyzed)
	}
}

func renderTemplate(t *testing.T, pd PageData) string {
	t.Helper()
	tmpl := template.Must(template.New("index").Funcs(template.FuncMap{
		"add": func(a, b int) int { return a + b },
	}).Parse(htmlTemplate))
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, pd); err != nil {
		t.Fatalf("template execution failed: %v", err)
	}
	return buf.String()
}

func TestScanSummaryTemplateRenders(t *testing.T) {
	out := renderTemplate(t, PageData{
		ScanSummary: &ScanSummaryView{Scanned: 20, Analyzed: 6, Archived: 2, Empty: 3, Excluded: 5, Skipped: 4},
	})
	if !strings.Contains(out, "Scan Summary") {
		t.Error("rendered page should contain the Scan Summary heading")
	}
	for _, label := range []string{"Scanned", "Analyzed", "Archived", "Empty", "Excluded", "Skipped"} {
		if !strings.Contains(out, label) {
			t.Errorf("rendered page missing %q tile", label)
		}
	}
}

func TestScanSummaryTemplateOmittedWhenNil(t *testing.T) {
	out := renderTemplate(t, PageData{})
	if strings.Contains(out, "Scan Summary") {
		t.Error("Scan Summary section should be omitted when no summary is present")
	}
}
