package utils

import (
	"github.com/jung-kurt/gofpdf"
)

// languageLineH is the height of the line listing a repository's top languages, drawn
// under its row in the PDF tables.
const languageLineH = 4.5

// languageSeparator separates the entries of a language line. The middle dot is in
// Windows-1252, so the core fonts render it once passed through tr.
const languageSeparator = " · "

// languageFont is the font a language line is drawn in.
type languageFont struct {
	Family string
	Size   float64
}

// lineBox places a language line: its top-left corner, its width, and whether its
// background is filled to match the row above it.
type lineBox struct {
	X, Y, W float64
	Fill    bool
}

// drawLanguageLine draws a repository's top languages on one line - "Go 1.00K · Kubernetes
// 100 · YAML 300" - across the given width, as the results page lists them. A language
// left out of the repository's Code Lines, globally or by the repository itself, is struck
// through and greyed, so the figure in the row above visibly omits it.
//
// Languages that do not fit are dropped whole and replaced by an ellipsis, rather than one
// being cut part-way: a half name with its line count missing would read as a different
// language. A dash when the repository has no language data.
func drawLanguageLine(pdf *gofpdf.Fpdf, tr func(string) string, langs []LanguageShare, font languageFont, box lineBox) {
	family, size := font.Family, font.Size
	x, y, w := box.X, box.Y, box.W
	// Set before the background cell: a cell cannot be drawn without a font, and this must
	// not depend on what the caller last left selected.
	pdf.SetFont(family, "", size)
	pdf.SetXY(x, y)
	pdf.CellFormat(w, languageLineH, "", "", 0, "L", box.Fill, 0, "")

	const pad = 2.0
	cursor := x + pad
	limit := x + w - pad

	write := func(text, style string, r, g, b int) {
		pdf.SetFont(family, style, size)
		pdf.SetTextColor(r, g, b)
		width := pdf.GetStringWidth(text)
		pdf.SetXY(cursor, y)
		pdf.CellFormat(width, languageLineH, text, "", 0, "L", false, 0, "")
		cursor += width
	}

	if len(langs) == 0 {
		write("-", "", 140, 140, 150)
		pdf.SetTextColor(0, 0, 0)
		return
	}

	pdf.SetFont(family, "", size)
	ellipsisW := pdf.GetStringWidth("...")
	sepW := pdf.GetStringWidth(tr(languageSeparator))

	for i, lang := range langs {
		text := tr(lang.Language + " " + lang.CodeLinesF)
		style := ""
		if lang.Excluded {
			style = "S"
		}
		pdf.SetFont(family, style, size)
		needed := pdf.GetStringWidth(text)
		if i > 0 {
			needed += sepW
		}
		// Leave room for the ellipsis unless this is the last entry.
		room := limit - cursor
		if i < len(langs)-1 {
			room -= ellipsisW
		}
		if needed > room {
			write("...", "", 140, 140, 150)
			break
		}

		if i > 0 {
			write(tr(languageSeparator), "", 140, 140, 150)
		}
		if lang.Excluded {
			write(text, "S", 150, 150, 160)
		} else {
			write(text, "", 60, 60, 70)
		}
	}
	pdf.SetTextColor(0, 0, 0)
}
