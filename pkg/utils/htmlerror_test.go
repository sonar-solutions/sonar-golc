package utils

import (
	"strings"
	"testing"
)

// iisUnauthorizedPage is the shape of what an IIS 401.1 response actually carries: a long
// stylesheet, a heading, the diagnostic fields, and a list of links. Roughly 6 KB of it ends
// up as the reason a repository was skipped.
const iisUnauthorizedPage = `<!DOCTYPE html PUBLIC "-//W3C//DTD XHTML 1.0 Strict//EN">
<html xmlns="http://www.w3.org/1999/xhtml">
<head>
<title>IIS 8.5 Detailed Error - 401.1 - Unauthorized</title>
<style type="text/css">
<!--
body{margin:0;font-size:.7em;font-family:Verdana,Arial,Helvetica,sans-serif;background:#CBE1EF;}
code{margin:0;color:#006600;font-size:1.1em;font-weight:bold;}
.config_source code{font-size:.8em;color:#000000;}
pre{margin:0;font-size:1.4em;word-wrap:break-word;}
ul,ol{margin:10px 0 10px 5px;}
-->
</style>
</head>
<body>
<div id="content">
<div class="content-container"><h3>HTTP Error 401.1 - Unauthorized</h3>
<h4>You do not have permission to view this directory or page using the credentials that you supplied.</h4>
</div>
<div class="content-container">
<fieldset><h4>Most likely causes:</h4>
<ul><li>No authentication protocol (including anonymous) is selected in IIS.</li>
<li>Only integrated authentication is enabled, and a client browser was used that does not support integrated authentication.</li></ul>
</fieldset>
</div>
<div class="content-container">
<fieldset><h4>Detailed Error Information:</h4>
<div id="details-left"><table border="0" cellpadding="0" cellspacing="0">
<tr class="alt"><th>Module</th><td>WindowsAuthenticationModule</td></tr>
<tr><th>Notification</th><td>AuthenticateRequest</td></tr>
<tr class="alt"><th>Handler</th><td>ExtensionlessUrlHandler</td></tr>
<tr><th>Error Code</th><td>0x80090308</td></tr>
</table></div>
</fieldset>
</div>
</div>
</body></html>`

// The point of the exercise: what lands in analysis_skipped.json has to be short enough to
// read. The page is ~2 KB even in this trimmed form; a real one is ~6 KB.
func TestCondenseHTMLErrorShrinksAnIISPage(t *testing.T) {
	raw := "clone failed: authentication required: " + iisUnauthorizedPage

	got := CondenseHTMLError(raw)

	if len(got) >= len(raw)/4 {
		t.Errorf("condensed to %d bytes from %d, which is not enough of a reduction to make "+
			"the skip report readable:\n%s", len(got), len(raw), got)
	}
	if strings.Contains(got, "font-family") || strings.Contains(got, "margin:0") {
		t.Errorf("stylesheet text survived:\n%s", got)
	}
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("markup survived:\n%s", got)
	}
}

// Condensing must not throw away the diagnosis. The message's own text says what failed, and
// the page's heading says why - both have to reach the report, or the entry becomes "something
// went wrong" and the operator has nowhere to go.
func TestCondenseHTMLErrorKeepsTheDiagnosis(t *testing.T) {
	got := CondenseHTMLError("clone failed: authentication required: " + iisUnauthorizedPage)

	for _, want := range []string{"clone failed: authentication required:", "401.1"} {
		if !strings.Contains(got, want) {
			t.Errorf("condensed reason lost %q:\n%s", want, got)
		}
	}
}

// The overwhelming majority of skip reasons are ordinary sentences. Rewriting those could only
// lose information, so they must come back byte-identical.
func TestCondenseHTMLErrorLeavesPlainMessagesAlone(t *testing.T) {
	messages := []string{
		"clone timed out after 5m0s",
		"analysis error: repository is empty",
		"clone failed: authentication required",
		"",
		"a message mentioning a < character and a > one",
	}

	for _, message := range messages {
		if got := CondenseHTMLError(message); got != message {
			t.Errorf("CondenseHTMLError(%q) = %q, want it unchanged", message, got)
		}
	}
}

// Script contents are not markup either, so stripping tags alone would leave the JavaScript
// behind as text.
func TestCondenseHTMLErrorDropsScriptContents(t *testing.T) {
	got := CondenseHTMLError(`<html><head><script>var x = 1; alert("boom");</script></head>` +
		`<body><h3>Access denied</h3></body></html>`)

	if strings.Contains(got, "alert") || strings.Contains(got, "var x") {
		t.Errorf("script text survived:\n%s", got)
	}
	if !strings.Contains(got, "Access denied") {
		t.Errorf("the message itself was lost:\n%s", got)
	}
}

// A page whose visible text is empty must still produce a reason. An empty string in the
// report reads as "no reason was recorded", which is a different and misleading claim.
func TestCondenseHTMLErrorNeverReturnsAnEmptyReason(t *testing.T) {
	got := CondenseHTMLError("<html><head><style>body{color:red}</style></head><body></body></html>")

	if strings.TrimSpace(got) == "" {
		t.Error("an HTML page with no text produced an empty reason")
	}
}

// Entities are how these pages write punctuation; leaving them raw makes the line harder to
// read than it needs to be.
func TestCondenseHTMLErrorDecodesEntities(t *testing.T) {
	got := CondenseHTMLError(`<html><body><h3>Don&#39;t have permission &amp; cannot proceed</h3></body></html>`)

	if !strings.Contains(got, "Don't have permission & cannot proceed") {
		t.Errorf("entities were not decoded:\n%s", got)
	}
}

// A page that is all text and no boilerplate still has to be capped - the limit is what bounds
// the report's size, not the presence of a stylesheet.
func TestCondenseHTMLErrorCapsLongText(t *testing.T) {
	got := CondenseHTMLError("<html><body>" + strings.Repeat("verbose ", 500) + "</body></html>")

	if len(got) > condensedHTMLLimit+10 {
		t.Errorf("condensed text is %d bytes, want it capped near %d", len(got), condensedHTMLLimit)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncation was not signalled, so the text reads as complete:\n%s", got)
	}
}

// The status code sits in the heading; the error *code* sits in a details table near the
// bottom. A length cap therefore keeps the vague half and drops the precise one - but
// 0x80090308 (SEC_E_INVALID_TOKEN, a broken handshake) versus 0xC000006A (a wrong password)
// is the difference between checking the auth configuration and checking the credentials.
func TestCondenseHTMLErrorKeepsTheErrorCodePastTheCap(t *testing.T) {
	got := CondenseHTMLError("clone failed: " + iisUnauthorizedPage)

	if !strings.Contains(got, "0x80090308") {
		t.Errorf("the error code was truncated away, leaving nothing to distinguish a broken "+
			"handshake from a rejected password:\n%s", got)
	}
}
