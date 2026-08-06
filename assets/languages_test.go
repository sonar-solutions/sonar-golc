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
	// Captured from a live instance via /api/settings/list_definitions - every
	// sonar.<lang>.file.suffixes default. Where GoLC splits a language that SonarQube
	// keeps whole (C/C++ headers, Scss under CSS) the suffixes are asserted on whichever
	// GoLC entry owns them.
	want := map[string][]string{
		"Gosu":       {".gs", ".gsx", ".gsp"},
		"Groovy":     {".groovy", ".gvy", ".gy", ".gsh", "Jenkinsfile"},
		"JSP":        {".jsp", ".jspf", ".jspx"},
		"PowerShell": {".ps1", ".psm1", ".psd1"},
		// Aligned to SonarQube's defaults; each of these was previously absent, so a
		// repository using it was silently under-counted.
		"C#":            {".cs", ".razor"},
		"C++":           {".cpp", ".cc", ".cxx", ".c++", ".ipp", ".ixx", ".mxx", ".cppm", ".ccm", ".cxxm", ".c++m"},
		"C++ Header":    {".hh", ".hpp", ".hxx", ".h++"},
		"CSS":           {".css"},
		"Less":          {".less"},
		"Sass":          {".sass"},
		"Twig":          {".twig"},
		"JavaScript":    {".js", ".jsx", ".cjs", ".mjs"},
		"TypeScript":    {".ts", ".tsx", ".cts", ".mts"},
		"Oracle PL/SQL": {".pkb", ".pks"},
		"RPG":           {".rpg", ".rpgle", ".sqlrpgle", ".RPG", ".RPGLE", ".SQLRPGLE"},
		"VB6":           {".bas", ".frm", ".ctl"},
		"XML":           {".xml", ".xsd", ".xsl", ".config"},
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

// Every language must be reachable. Normally that means declaring an extension; the IaC
// dialects are the exception, since they are recognised from file content by
// analyzer.RefineLanguage and would collide with YAML/JSON if they claimed a suffix.
// The two conditions are mutually exclusive, so a mistake in either direction is caught.
func TestEveryLanguageIsReachable(t *testing.T) {
	for lang, info := range Languages {
		switch {
		case info.ContentDetected && len(info.Extensions) > 0:
			t.Errorf("language %q is content-detected but also claims extensions %v; it "+
				"would be resolved by suffix and shadow YAML or JSON", lang, info.Extensions)
		case !info.ContentDetected && len(info.Extensions) == 0:
			t.Errorf("language %q declares no extensions and is not content-detected, so "+
				"it can never be counted", lang)
		}
	}
}

// The IaC dialects must inherit the comment syntax of the format they are written in,
// otherwise their comment lines would be miscounted as code.
func TestContentDetectedLanguagesInheritHostCommentSyntax(t *testing.T) {
	yamlBased := map[string]bool{
		"Ansible": true, "Azure Pipelines": true, "CloudFormation": true,
		"GitHub Actions": true, "Kubernetes": true,
	}

	for lang, info := range Languages {
		if !info.ContentDetected {
			continue
		}
		hasHash := len(info.LineComments) == 1 && info.LineComments[0] == "#"
		if yamlBased[lang] && !hasHash {
			t.Errorf("%q is YAML-based and must treat # as a line comment, got %v",
				lang, info.LineComments)
		}
		if !yamlBased[lang] && len(info.LineComments) != 0 {
			t.Errorf("%q is JSON-based and JSON has no comment syntax, got %v",
				lang, info.LineComments)
		}
	}
}

// The delimiter rule exists to match SonarQube's ncloc, which ignores a line holding only
// a PHP tag. Whole-line matching is what makes "<?php $a = 1;" still count as code.
func TestPHPDeclaresItsMarkupDelimiters(t *testing.T) {
	want := map[string]bool{"<?php": false, "<?": false, "?>": false}
	for _, d := range Languages["PHP"].NonCodeLines {
		if _, ok := want[d]; !ok {
			t.Errorf("unexpected PHP delimiter %q", d)
			continue
		}
		want[d] = true
	}
	for d, found := range want {
		if !found {
			t.Errorf("PHP should declare %q as a non-code delimiter", d)
		}
	}
}

// Less, Sass and Twig are reported by SonarQube under css and html, but they cannot share
// GoLC's CSS or HTML entry: Less and Sass support "//" line comments that plain CSS does
// not, and a Twig comment is {# #}. Folding them in would count their comments as code -
// over-counting, which is the opposite of what aligning with ncloc is for.
func TestTemplateAndPreprocessorCommentSyntax(t *testing.T) {
	for _, lang := range []string{"Less", "Sass"} {
		info, ok := Languages[lang]
		if !ok {
			t.Errorf("%q is missing", lang)
			continue
		}
		if len(info.LineComments) != 1 || info.LineComments[0] != "//" {
			t.Errorf("%q must treat // as a line comment, got %v", lang, info.LineComments)
		}
	}

	// Plain CSS has no line comment syntax, so it must not have acquired one.
	if len(Languages["CSS"].LineComments) != 0 {
		t.Errorf("CSS has no line comments, got %v", Languages["CSS"].LineComments)
	}

	var hasTwigComment bool
	for _, pair := range Languages["Twig"].MultiLineComments {
		if len(pair) == 2 && pair[0] == "{#" && pair[1] == "#}" {
			hasTwigComment = true
		}
	}
	if !hasTwigComment {
		t.Errorf("Twig should recognise {# #} comments, got %v",
			Languages["Twig"].MultiLineComments)
	}
}
