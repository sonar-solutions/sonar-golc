package assets

import (
	"sort"
	"testing"
)

// extensionOwners maps each configured extension to the language(s) claiming it.
func extensionOwners() map[string][]string {
	owners := map[string][]string{}
	for lang, info := range Languages {
		for _, extension := range info.Extensions {
			owners[extension] = append(owners[extension], lang)
		}
	}
	for _, langs := range owners {
		sort.Strings(langs)
	}
	return owners
}

// The counts GoLC reports are only as good as this map, so the languages SonarQube
// analyses by default should be present with the same suffixes it uses. These were
// missing entirely, which made GoLC under-report repositories that use them.
func TestLanguagesCoverSonarQubeDefaults(t *testing.T) {
	want := map[string][]string{
		"Gosu":       {".gs", ".gsx", ".gsp"},
		"Groovy":     {".groovy", ".gvy", ".gy", ".gsh", "Jenkinsfile"},
		"JSP":        {".jsp", ".jspf", ".jspx"},
		"PowerShell": {".ps1", ".psm1", ".psd1"},
	}

	for lang, extensions := range want {
		info, ok := Languages[lang]
		if !ok {
			t.Errorf("language %q is missing from the map", lang)
			continue
		}
		have := map[string]bool{}
		for _, e := range info.Extensions {
			have[e] = true
		}
		for _, e := range extensions {
			if !have[e] {
				t.Errorf("language %q does not claim extension %q", lang, e)
			}
		}
	}
}

// .jsp and .jspf used to be listed under JavaScript, which reported them as JavaScript
// and applied "//" line comments to a syntax that has none.
func TestJSPIsNotJavaScript(t *testing.T) {
	for _, extension := range Languages["JavaScript"].Extensions {
		if extension == ".jsp" || extension == ".jspf" || extension == ".jspx" {
			t.Errorf("JavaScript still claims %q; it belongs to JSP", extension)
		}
	}

	jsp, ok := Languages["JSP"]
	if !ok {
		t.Fatal("JSP language is missing")
	}
	if len(jsp.LineComments) != 0 {
		t.Errorf("JSP has no line comment syntax, got %v", jsp.LineComments)
	}
	var hasJSPComment bool
	for _, pair := range jsp.MultiLineComments {
		if len(pair) == 2 && pair[0] == "<%--" && pair[1] == "--%>" {
			hasJSPComment = true
		}
	}
	if !hasJSPComment {
		t.Errorf("JSP should recognise <%%-- --%%> comments, got %v", jsp.MultiLineComments)
	}
}

// An extension claimed by two languages is resolved by iterating a Go map, so the label
// GoLC reports for it is not deterministic. Two such collisions predate this test and
// are accepted; the point is to stop new ones being introduced unnoticed.
func TestNoNewExtensionCollisions(t *testing.T) {
	accepted := map[string]bool{
		".as":  true, // ActionScript / Flex
		".cls": true, // Apex / VB6
	}

	for extension, langs := range extensionOwners() {
		if len(langs) > 1 && !accepted[extension] {
			t.Errorf("extension %q is claimed by %v; the reported language would be "+
				"whichever wins the map iteration", extension, langs)
		}
	}
}

// Every language must be usable: a language with no extensions can never match a file.
func TestEveryLanguageHasAnExtension(t *testing.T) {
	for lang, info := range Languages {
		if len(info.Extensions) == 0 {
			t.Errorf("language %q declares no extensions, so it can never be counted", lang)
		}
	}
}
