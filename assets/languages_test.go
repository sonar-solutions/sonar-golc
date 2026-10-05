package assets

import (
	"sort"
	"strings"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/pkg/analyzer"
)

// extensionOwners maps each configured extension to the language(s) claiming it.
func extensionOwners() map[string][]string {
	owners := map[string][]string{}
	for lang, info := range Languages {
		for _, extension := range info.Extensions {
			key := analyzer.ExtensionKey(extension)
			owners[key] = append(owners[key], lang)
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
		"RPG":           {".rpg", ".rpgle", ".sqlrpgle"},
		"VB6":           {".bas", ".frm", ".ctl"},
		"XML":           {".xml", ".xsd", ".xsl", ".config"},
		// Added for SonarQube 2026.5, which analyses each of these by default.
		"R":                 {".r"},
		"Apex":              {".cls", ".trigger", ".apex"},
		"DataWeave":         {".dwl"},
		"PostgreSQL":        {".pgsql", ".psql"},
		"Bicep":             {".bicep"},
		"Docker":            {"Dockerfile", "dockerfile", ".dockerfile", "Containerfile", "containerfile", ".containerfile"},
		"IPython Notebooks": {".ipynb"},
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
// GoLC reports for it would not be deterministic. The last two collisions - .cls
// (Apex / VB6) and .as (ActionScript / Flex) - were resolved the way SonarQube resolves
// them, and none may come back.
func TestNoExtensionCollisions(t *testing.T) {
	for extension, langs := range extensionOwners() {
		if len(langs) > 1 {
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

// sonar.r.file.suffixes lists Rmd and rmd, yet SonarQube reports no lines of code for R
// Markdown, so claiming those suffixes would over-count.
func TestRMarkdownIsNotCounted(t *testing.T) {
	for extension, langs := range extensionOwners() {
		if extension == ".Rmd" || extension == ".rmd" {
			t.Errorf("%q is claimed by %v; SonarQube counts no lines for R Markdown", extension, langs)
		}
	}
}

// Notebooks are the only language counted from inside a JSON document; any other language
// flagged as one would have its source files parsed as JSON and counted as empty.
func TestOnlyNotebooksAreJupyterNotebooks(t *testing.T) {
	for lang, info := range Languages {
		if info.JupyterNotebook != (lang == "IPython Notebooks") {
			t.Errorf("%q has JupyterNotebook = %v", lang, info.JupyterNotebook)
		}
	}
}

// Suffixes are looked up in lower case, so an uppercase spelling in the table could never
// match anything; listing one would only suggest that case still matters.
func TestSuffixesAreLowerCase(t *testing.T) {
	for lang, info := range Languages {
		for _, extension := range info.Extensions {
			if strings.HasPrefix(extension, ".") && extension != strings.ToLower(extension) {
				t.Errorf("%q lists %q; suffixes are matched regardless of case, so list it "+
					"in lower case", lang, extension)
			}
		}
	}
}

// These suffixes were counted by GoLC but are not in SonarQube's defaults, so a stock
// SonarQube reports no lines for them. Counting them over-reported the repositories that
// have them.
func TestSuffixesSonarQubeDoesNotAnalyseAreNotCounted(t *testing.T) {
	notAnalysed := map[string]string{
		".zsh": "Shell", ".ksh": "Shell", ".fish": "Shell",
		".job": "JCL", ".jjob": "JCL",
		".pl1": "PL/I",
		".mm":  "Objective-C",
	}

	owners := extensionOwners()
	for extension, was := range notAnalysed {
		if langs, ok := owners[extension]; ok {
			t.Errorf("%q is claimed by %v (formerly %s); SonarQube does not analyse it by default",
				extension, langs, was)
		}
	}

	// .cls belongs to Apex alone: SonarQube's VB6 suffixes are .bas, .frm and .ctl.
	if langs := owners[".cls"]; len(langs) != 1 || langs[0] != "Apex" {
		t.Errorf(".cls is claimed by %v, want [Apex]", langs)
	}
}
