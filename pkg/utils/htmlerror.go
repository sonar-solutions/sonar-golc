package utils

import (
	"html"
	"regexp"
	"strings"
)

// A server that refuses a request often answers with a whole error page rather than a
// sentence. IIS is the worst offender - its 401.1 page is roughly 6 KB of stylesheet, list
// items and links to knowledge-base articles - and when that page arrives as the text of a
// clone failure it is stored verbatim as the reason the repository was skipped.
//
// Two skipped repositories were enough to turn analysis_skipped.json into 12 KB of mostly CSS.
// That file is read by the web page and the PDF report, so the one thing an operator needs
// from it - which repositories are missing from the totals, and why - ends up buried in
// markup. Condensing the page to its visible words keeps the diagnosis and drops the rest.

var (
	htmlStyleBlock  = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	htmlScriptBlock = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	htmlTag         = regexp.MustCompile(`(?s)<[^>]*>`)
	htmlWhitespace  = regexp.MustCompile(`\s+`)
	htmlErrorCode   = regexp.MustCompile(`0x[0-9A-Fa-f]{8}`)
)

// condensedHTMLLimit caps the extracted text. Enough for a heading and an error code, which is
// what these pages carry that is worth reading; anything longer is boilerplate.
const condensedHTMLLimit = 200

// CondenseHTMLError rewrites an error message that carries an embedded HTML page, keeping the
// message's own text and reducing the page to its visible words. A message with no HTML in it
// is returned exactly as it was - the common case, and one where any rewriting would only lose
// information.
func CondenseHTMLError(message string) string {
	start := indexOfHTML(message)
	if start < 0 {
		return message
	}

	prefix := strings.TrimSpace(message[:start])
	condensed := condenseHTML(message[start:])

	switch {
	case prefix == "" && condensed == "":
		// An HTML page with no readable text at all. Saying so beats an empty reason, which
		// would read as though no reason was recorded.
		return "server returned an HTML error page with no readable message"
	case prefix == "":
		return condensed
	case condensed == "":
		return prefix
	default:
		return prefix + " " + condensed
	}
}

// indexOfHTML returns where an HTML document starts within s, or -1.
func indexOfHTML(s string) int {
	lower := strings.ToLower(s)

	first := -1
	for _, marker := range []string{"<!doctype", "<html"} {
		if i := strings.Index(lower, marker); i >= 0 && (first < 0 || i < first) {
			first = i
		}
	}

	return first
}

// condenseHTML reduces a page to a single line of its visible text.
func condenseHTML(page string) string {
	// Style and script contents are not markup, so stripping tags alone would leave the CSS
	// and JavaScript behind as text - which is most of what makes these pages long.
	page = htmlStyleBlock.ReplaceAllString(page, " ")
	page = htmlScriptBlock.ReplaceAllString(page, " ")
	page = htmlTag.ReplaceAllString(page, " ")

	text := strings.TrimSpace(htmlWhitespace.ReplaceAllString(html.UnescapeString(page), " "))
	if len(text) <= condensedHTMLLimit {
		return text
	}

	truncated := strings.TrimSpace(text[:condensedHTMLLimit]) + "..."

	// These pages put the status code in the heading and the actual status *code* far down in
	// a details table, so a length cap reliably keeps the vague half and discards the precise
	// one. 0x80090308 is what distinguishes a rejected credential from a broken handshake -
	// the difference between "check the password" and "check the auth configuration" - so it
	// is carried past the cap rather than left to fall off the end.
	if code := htmlErrorCode.FindString(text); code != "" && !strings.Contains(truncated, code) {
		truncated += " (" + code + ")"
	}

	return truncated
}
