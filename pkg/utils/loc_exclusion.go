package utils

import (
	"encoding/json"
	"fmt"
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
//
// On top of that global set, individual repositories can exclude further languages of
// their own. A repository can only exclude more, never count a language the global set
// excludes: "why is YAML counted in this one repository?" would be hard to explain in a
// report. Code that counts one repository's lines takes ForRepo(key), whose Excludes
// then answers for that repository.
type LanguageExclusion struct {
	set map[string]bool
	// perRepo maps a repository key to the languages it excludes beyond the global set.
	perRepo map[string]map[string]bool
	// repo is the scoped repository's own set, filled in by ForRepo.
	repo map[string]bool
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

// Excludes reports whether a language is left out of the totals - everywhere, or in the
// repository this selection was scoped to with ForRepo.
func (e LanguageExclusion) Excludes(language string) bool {
	return e.ExcludesEverywhere(language) || e.repo[strings.TrimSpace(language)]
}

// ExcludedHere reports whether a language is excluded by the scoped repository's own
// selection rather than globally.
func (e LanguageExclusion) ExcludedHere(language string) bool {
	return e.repo[strings.TrimSpace(language)] && !e.ExcludesEverywhere(language)
}

// ExcludesEverywhere reports whether a language is in the global set, which applies to
// every repository.
func (e LanguageExclusion) ExcludesEverywhere(language string) bool {
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

// ForRepo returns the selection as it applies to one repository: the global set plus the
// languages that repository excludes of its own.
func (e LanguageExclusion) ForRepo(key string) LanguageExclusion {
	e.repo = e.perRepo[key]
	return e
}

// WithRepoExclusions returns the selection with each repository's own exclusions
// replaced by the given ones. Languages the global set already excludes are dropped - a
// repository cannot exclude them twice - as are repositories left with nothing.
func (e LanguageExclusion) WithRepoExclusions(byRepo map[string][]string) LanguageExclusion {
	e.perRepo = make(map[string]map[string]bool, len(byRepo))
	for key, langs := range byRepo {
		set := make(map[string]bool, len(langs))
		for _, lang := range langs {
			if name := strings.TrimSpace(lang); name != "" && !e.ExcludesEverywhere(name) {
				set[name] = true
			}
		}
		if len(set) > 0 {
			e.perRepo[key] = set
		}
	}
	e.repo = nil
	return e
}

// RepoExclusions returns each repository's own exclusions, languages sorted.
func (e LanguageExclusion) RepoExclusions() map[string][]string {
	out := make(map[string][]string, len(e.perRepo))
	for key, set := range e.perRepo {
		langs := make([]string, 0, len(set))
		for lang := range set {
			langs = append(langs, lang)
		}
		sort.Strings(langs)
		out[key] = langs
	}
	return out
}

// HasRepoExclusions reports whether any repository excludes languages of its own.
func (e LanguageExclusion) HasRepoExclusions() bool {
	return len(e.perRepo) > 0
}

// Fingerprint identifies the whole selection, per-repository exclusions included, for
// deciding whether a report built under one selection is still current.
func (e LanguageExclusion) Fingerprint() string {
	parts := []string{"global", strings.Join(e.Languages(), ",")}
	byRepo := e.RepoExclusions()
	keys := make([]string, 0, len(byRepo))
	for key := range byRepo {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		parts = append(parts, key, strings.Join(byRepo[key], ","))
	}
	return strings.Join(parts, "\x00")
}

// Languages returns the globally excluded languages, sorted.
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

// IsDefault reports whether the selection is the default one, however it was built: the
// default global set, and no repository excluding anything of its own.
func (e LanguageExclusion) IsDefault() bool {
	return e.GlobalIsDefault() && !e.HasRepoExclusions()
}

// GlobalIsDefault reports whether the global set is the default one, whatever individual
// repositories exclude.
func (e LanguageExclusion) GlobalIsDefault() bool {
	return strings.Join(e.Languages(), "\x00") == strings.Join(DefaultLanguageExclusion().Languages(), "\x00")
}

// Matches reports whether a total recorded as leaving out the given languages was
// counted under this selection. A nil record never matches: it is a GlobalReport.json
// written before the scanner recorded its selection, whose total may still include
// plain YAML, so its figures have to be recounted rather than trusted.
//
// The scanner records only a global set, so a selection with per-repository exclusions
// never matches either.
func (e LanguageExclusion) Matches(recorded []string) bool {
	if recorded == nil || e.HasRepoExclusions() {
		return false
	}
	return strings.Join(NewLanguageExclusion(recorded).Languages(), "\x00") == strings.Join(e.Languages(), "\x00")
}

// Note is the sentence shown beside every total, saying which languages it leaves out.
func (e LanguageExclusion) Note() string {
	note := e.globalNote()
	if n := len(e.perRepo); n == 1 {
		note += " 1 repository also excludes languages of its own."
	} else if n > 1 {
		note += fmt.Sprintf(" %d repositories also exclude languages of their own.", n)
	}
	return note
}

// RepoNote is the note for one repository's own page, on a selection scoped with ForRepo:
// the global set, then the languages this repository excludes of its own by name.
func (e LanguageExclusion) RepoNote() string {
	note := e.globalNote()
	if len(e.repo) == 0 {
		return note
	}
	langs := make([]string, 0, len(e.repo))
	for lang := range e.repo {
		if !e.ExcludesEverywhere(lang) {
			langs = append(langs, lang)
		}
	}
	sort.Strings(langs)
	if len(langs) == 0 {
		return note
	}
	return note + " This repository also excludes " + joinLanguages(langs) + "."
}

// globalNote is the sentence describing the global set alone.
func (e LanguageExclusion) globalNote() string {
	langs := e.Languages()
	switch {
	case len(langs) == 0:
		return "All languages are counted in the total."
	case e.GlobalIsDefault():
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

// LoadLanguageExclusion reads the selection for a base Results directory, per-repository
// exclusions included. A missing or unreadable file yields the defaults, so a result set
// that predates this feature - or one the user never touched - reports exactly what it
// always did.
func LoadLanguageExclusion(baseResultsDir string) LanguageExclusion {
	return loadGlobalExclusion(baseResultsDir).WithRepoExclusions(loadRepoExclusions(baseResultsDir))
}

// loadGlobalExclusion reads the global set alone.
func loadGlobalExclusion(baseResultsDir string) LanguageExclusion {
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

// ClearLanguageExclusion removes a persisted selection, per-repository exclusions
// included, so the next reports use the defaults again. A fresh analysis calls this: the
// selection was made against the previous scan's languages and repositories, and every
// run starts from SonarQube's defaults.
func ClearLanguageExclusion(baseResultsDir string) error {
	for _, path := range []string{ExcludedLanguagesPath(baseResultsDir), RepoExcludedLanguagesPath(baseResultsDir)} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// repoExcludedLanguagesReport is the on-disk envelope persisted to
// Results/config/excluded_repo_languages.json.
type repoExcludedLanguagesReport struct {
	Repositories map[string][]string `json:"Repositories"`
}

// RepoExcludedLanguagesPath returns the canonical location of the per-repository
// exclusions for a given base Results directory.
func RepoExcludedLanguagesPath(baseResultsDir string) string {
	return filepath.Join(baseResultsDir, "config", "excluded_repo_languages.json")
}

// SaveRepoLanguageExclusions writes each repository's own exclusions. The global set is
// saved separately by SaveLanguageExclusion, so either can change without rewriting the
// other.
func SaveRepoLanguageExclusions(baseResultsDir string, e LanguageExclusion) error {
	path := RepoExcludedLanguagesPath(baseResultsDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(repoExcludedLanguagesReport{Repositories: e.RepoExclusions()}, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// loadRepoExclusions reads the per-repository exclusions; a missing or unreadable file
// means no repository excludes anything of its own.
func loadRepoExclusions(baseResultsDir string) map[string][]string {
	data, err := os.ReadFile(RepoExcludedLanguagesPath(baseResultsDir))
	if err != nil {
		return nil
	}
	var rep repoExcludedLanguagesReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil
	}
	return rep.Repositories
}
