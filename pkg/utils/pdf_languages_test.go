package utils

import (
	"bytes"
	"strings"
	"testing"

	"github.com/jung-kurt/gofpdf"
)

// languageLinePDF draws one language line into an uncompressed PDF and returns its bytes,
// so the drawing operators can be inspected.
func languageLinePDF(t *testing.T, langs []LanguageShare, width float64) string {
	t.Helper()
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetCompression(false)
	pdf.AddPage()
	tr := pdf.UnicodeTranslatorFromDescriptor("")
	drawLanguageLine(pdf, tr, langs, languageFont{"Helvetica", 7}, lineBox{X: 15, Y: 20, W: width})
	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		t.Fatalf("pdf output: %v", err)
	}
	return buf.String()
}

func TestLanguageLineStrikesThroughExcludedLanguages(t *testing.T) {
	counted := []LanguageShare{{Language: "Go", CodeLinesF: "1.00K"}, {Language: "YAML", CodeLinesF: "300"}}
	struck := []LanguageShare{{Language: "Go", CodeLinesF: "1.00K"}, {Language: "YAML", CodeLinesF: "300", Excluded: true}}

	plain := languageLinePDF(t, counted, 180)
	withStrike := languageLinePDF(t, struck, 180)
	for _, want := range []string{"(Go 1.00K)", "(YAML 300)"} {
		if !strings.Contains(withStrike, want) {
			t.Errorf("language line missing %s", want)
		}
	}
	// gofpdf draws a strike-through as a filled rectangle across the text: the excluded
	// entry adds exactly one.
	if got, want := strings.Count(withStrike, " re f"), strings.Count(plain, " re f")+1; got != want {
		t.Errorf("found %d filled rectangles, want %d - the excluded language should be struck through", got, want)
	}
}

func TestLanguageLineDropsWholeEntriesThatDoNotFit(t *testing.T) {
	langs := []LanguageShare{
		{Language: "Go", CodeLinesF: "1.00K"}, {Language: "JavaScript", CodeLinesF: "900"},
		{Language: "Kubernetes", CodeLinesF: "800"}, {Language: "CloudFormation", CodeLinesF: "700"},
		{Language: "Python", CodeLinesF: "600"},
	}
	out := languageLinePDF(t, langs, 50)
	if !strings.Contains(out, "(Go 1.00K)") || !strings.Contains(out, "(...)") {
		t.Error("a narrow line should keep the first entries whole and end with an ellipsis")
	}
	if strings.Contains(out, "(Python 600)") {
		t.Error("entries that do not fit should be dropped, not squeezed in")
	}
	if dash := languageLinePDF(t, nil, 50); !strings.Contains(dash, "(-)") {
		t.Error("a repository without language data should show a dash")
	}
}
