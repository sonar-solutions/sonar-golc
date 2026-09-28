package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultExcludedLanguages are the languages held out of report totals unless the user
// changes the selection on the results page.
//
// Both reproduce SonarQube's default configuration. Plain JSON and YAML analysis ship off
// (sonar.json.activate and sonar.yaml.activate default to false on SonarQube Server, and on
// SonarQube Cloud the generic YAML/JSON sensor also waits on a feature flag), so a stock
// SonarQube counts none of their lines. The infrastructure-as-code dialects of both formats
// are reported under their own names (Kubernetes, CloudFormation, ...) and stay counted,
// because every IaC analyzer is on by default - see analyzer.RefineLanguage.
var DefaultExcludedLanguages = []string{"JSON", "YAML"}

// LanguageExclusion is the set of languages whose lines of code are left out of every
// report total. The zero value is the default set, so code that never loads a selection
// keeps reproducing SonarQube's defaults; an explicit empty selection counts everything.
type LanguageExclusion struct {
	set map[string]bool
}

// DefaultLanguageExclusion returns the default selection.
func DefaultLanguageExclusion() LanguageExclusion {
	return LanguageExclusion{}
}

// NewLanguageExclusion returns a selection excluding exactly the given languages. An
// empty list excludes nothing - which is different from the zero value.
func NewLanguageExclusion(languages []string) LanguageExclusion {
	set := make(map[string]bool, len(languages))
	for _, lang := range languages {
		if name := strings.TrimSpace(lang); name != "" {
			set[name] = true
		}
	}
	return LanguageExclusion{set: set}
}

// Excludes reports whether a language is left out of the totals.
func (e LanguageExclusion) Excludes(language string) bool {
	name := strings.TrimSpace(language)
	if e.set == nil {
		for _, lang := range DefaultExcludedLanguages {
			if lang == name {
				return true
			}
		}
		return false
	}
	return e.set[name]
}

// Languages returns the excluded languages, sorted.
func (e LanguageExclusion) Languages() []string {
	if e.set == nil {
		langs := append([]string(nil), DefaultExcludedLanguages...)
		sort.Strings(langs)
		return langs
	}
	langs := make([]string, 0, len(e.set))
	for lang := range e.set {
		langs = append(langs, lang)
	}
	sort.Strings(langs)
	return langs
}

// IsDefault reports whether the selection is the default one, however it was built.
func (e LanguageExclusion) IsDefault() bool {
	return strings.Join(e.Languages(), "\x00") == strings.Join(DefaultLanguageExclusion().Languages(), "\x00")
}

// Note is the sentence shown beside every total, saying which languages it leaves out.
func (e LanguageExclusion) Note() string {
	langs := e.Languages()
	switch {
	case len(langs) == 0:
		return "All languages are counted in the total."
	case e.IsDefault():
		return joinLanguages(langs) + " are excluded from the total to reproduce standard SonarQube behavior."
	default:
		return "Excluded from the total: " + joinLanguages(langs) + "."
	}
}

// joinLanguages renders a list as "A", "A and B" or "A, B and C".
func joinLanguages(langs []string) string {
	if len(langs) <= 1 {
		return strings.Join(langs, "")
	}
	return strings.Join(langs[:len(langs)-1], ", ") + " and " + langs[len(langs)-1]
}

// excludedLanguagesReport is the on-disk envelope persisted to
// Results/config/excluded_languages.json.
type excludedLanguagesReport struct {
	ExcludedLanguages []string `json:"ExcludedLanguages"`
}

// ExcludedLanguagesPath returns the canonical location of the language selection for a
// given base Results directory.
func ExcludedLanguagesPath(baseResultsDir string) string {
	return filepath.Join(baseResultsDir, "config", "excluded_languages.json")
}

// SaveLanguageExclusion writes the selection under <baseResultsDir>/config. An empty
// selection is written as an empty list, so "count everything" survives a reload instead
// of falling back to the defaults.
func SaveLanguageExclusion(baseResultsDir string, e LanguageExclusion) error {
	path := ExcludedLanguagesPath(baseResultsDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(excludedLanguagesReport{ExcludedLanguages: e.Languages()}, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadLanguageExclusion reads the selection for a base Results directory. A missing or
// unreadable file yields the defaults, so a result set that predates this feature - or
// one the user never touched - reports exactly what it always did.
func LoadLanguageExclusion(baseResultsDir string) LanguageExclusion {
	data, err := os.ReadFile(ExcludedLanguagesPath(baseResultsDir))
	if err != nil {
		return DefaultLanguageExclusion()
	}
	var rep excludedLanguagesReport
	if err := json.Unmarshal(data, &rep); err != nil || rep.ExcludedLanguages == nil {
		return DefaultLanguageExclusion()
	}
	return NewLanguageExclusion(rep.ExcludedLanguages)
}

// ClearLanguageExclusion removes a persisted selection so the next reports use the
// defaults again. A fresh analysis calls this: the selection was made against the
// previous scan's languages, and every run starts from SonarQube's defaults.
func ClearLanguageExclusion(baseResultsDir string) error {
	err := os.Remove(ExcludedLanguagesPath(baseResultsDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
