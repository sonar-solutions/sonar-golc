package utils

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Deselection is the post-scan counterpart to discovery-time exclusion: the
// repositories were cloned and counted, but the user removed them from the report
// totals afterwards on the results page (e.g. dead or vendored repositories that
// should not count towards a sizing exercise).
//
// It is deliberately kept distinct from ScanSummary.Excluded, which means
// "filtered out before analysis and never counted". Folding the two together
// would break the Scanned = Analyzed + Archived + Empty + Excluded + Skipped
// invariant that ScanSummary documents and every report relies on.
//
// Nothing is destroyed by a deselection: the per-repository result files under
// Results/{bylanguage,byfile}-report are never modified, so clearing the set and
// regenerating restores the original unfiltered reports exactly.

// keySeparator matches the `__` field separator used by result file names, so a
// deselection key is exactly the stem of the repository's result file. Using the
// pipeline's own identity means the key survives repositories, groups and
// branches that contain a single `_`.
const keySeparator = "__"

// SanitizeResultComponent normalizes one component of a result file name.
//
// Result file names embed the organization, repository and branch verbatim, so a
// component containing a path separator — a GitLab subgroup like `group/subgroup`, or
// a branch like `release/1.0` — would otherwise turn the file name into a nested path.
//
// Separators become `_`, deliberately matching what the reporters do when they write
// the file (see the `strings.Replace(OutputName, "/", "_", -1)` in
// pkg/reporter/{json,csv,pdf}). Deleting them instead would be equally safe against
// traversal but would name a file that is not the one on disk, so every repository
// with a subgroup or a slashed branch would silently go missing from the reports.
//
// A single `_` is unambiguous here because the fields are joined by `__`.
//
// Traversal is impossible once no separator remains, so `..` needs no special case:
// `../..` normalizes to `.._..`, an ordinary file name fragment.
//
// This is the one definition used for both path building and key building. While the
// results page and the report generators had separate rules, they derived different
// keys for the same repository and a deselection applied to one but not the other.
func SanitizeResultComponent(component string) string {
	component = strings.ReplaceAll(component, "/", "_")
	component = strings.ReplaceAll(component, "\\", "_")
	return strings.ReplaceAll(component, "\x00", "")
}

// DeselectedRepo identifies one repository (branch) removed from report totals.
// Key is authoritative for matching; Org, Repo and Branch exist so the reports can
// list what was removed without re-deriving it.
type DeselectedRepo struct {
	Key    string `json:"Key"`
	Org    string `json:"Org,omitempty"`
	Repo   string `json:"Repo"`
	Branch string `json:"Branch,omitempty"`
}

// DeselectedReport is the on-disk envelope persisted to
// Results/config/deselected_repos.json.
type DeselectedReport struct {
	DeselectedRepositories []DeselectedRepo `json:"DeselectedRepositories"`
}

// DeselectionSet is a lookup set of deselection keys.
type DeselectionSet map[string]bool

// DeselectionKey builds the key for a repository on a platform whose result files
// carry all three components (GitHub, GitLab, Azure, Bitbucket Cloud/DC). firstPart
// is the organization for GitHub/GitLab and the project key for Azure/Bitbucket —
// i.e. whatever getFirstPartForPlatform returns for that platform.
//
// Takes the raw inventory fields and normalizes them itself, so every caller gets the
// same key whether or not it sanitized beforehand.
func DeselectionKey(firstPart, repo, branch string) string {
	return SanitizeResultComponent(firstPart) + keySeparator +
		SanitizeResultComponent(repo) + keySeparator +
		SanitizeResultComponent(branch)
}

// FileDeselectionKey builds the key for the `file` platform, whose result files are
// named Result_<Repo>.json and therefore have no organization or branch component.
func FileDeselectionKey(repo string) string {
	return SanitizeResultComponent(repo)
}

// DeselectionKeyFromResultFileName derives the key from a result file name so a
// directory walk can filter without needing the analysis inventory. It accepts both
// the three-component platform form (Result_<Org>__<Repo>__<Branch>.json) and the
// single-component file-platform form (Result_<Repo>.json). Returns ok=false for
// names that are not result files at all.
func DeselectionKeyFromResultFileName(name string) (string, bool) {
	if org, repo, branch, ok := ParseResultFileName(name); ok {
		return DeselectionKey(org, repo, branch), true
	}
	if !strings.HasPrefix(name, "Result_") {
		return "", false
	}
	base := strings.TrimPrefix(name, "Result_")
	for _, suffix := range []string{".json", ".pdf"} {
		base = strings.TrimSuffix(base, suffix)
	}
	base = strings.TrimSuffix(base, "_byfile")
	if base == "" {
		return "", false
	}
	return FileDeselectionKey(base), true
}

// DeselectionKeyForRepo builds the key for a repository from its inventory fields,
// choosing the right form for the platform. The `file` platform writes single-component
// result file names, every other platform writes all three.
//
// Both the results page and the report generators call this, so the key cannot depend
// on how either of them happens to build file paths.
func DeselectionKeyForRepo(platform, firstPart, repo, branch string) string {
	if platform == "file" {
		return FileDeselectionKey(repo)
	}
	return DeselectionKey(firstPart, repo, branch)
}

// Contains reports whether a key is deselected. It is nil-safe so callers can pass
// an absent set without branching.
func (s DeselectionSet) Contains(key string) bool {
	if s == nil {
		return false
	}
	return s[key]
}

// DeselectionKeys builds a lookup set from a persisted list.
func DeselectionKeys(repos []DeselectedRepo) DeselectionSet {
	set := make(DeselectionSet, len(repos))
	for _, r := range repos {
		if r.Key != "" {
			set[r.Key] = true
		}
	}
	return set
}

// DeselectedReposPath returns the canonical location of the deselection file for a
// given base Results directory.
func DeselectedReposPath(baseResultsDir string) string {
	return filepath.Join(baseResultsDir, "config", "deselected_repos.json")
}

// SaveDeselectedRepos writes the deselection list under <baseResultsDir>/config.
// The file is always (re)written — including with an empty list — so resetting the
// selection clears any stale entries.
func SaveDeselectedRepos(baseResultsDir string, repos []DeselectedRepo) error {
	if repos == nil {
		repos = []DeselectedRepo{}
	}
	path := DeselectedReposPath(baseResultsDir)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(DeselectedReport{DeselectedRepositories: repos}, "", "    ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// LoadDeselectedRepos reads the deselection list for a base Results directory. A
// missing or unreadable file yields nil (not an error): every report must render
// fine on result sets that predate this feature, and an absent file simply means
// nothing was deselected.
func LoadDeselectedRepos(baseResultsDir string) []DeselectedRepo {
	data, err := os.ReadFile(DeselectedReposPath(baseResultsDir))
	if err != nil {
		return nil
	}
	var rep DeselectedReport
	if err := json.Unmarshal(data, &rep); err != nil {
		return nil
	}
	return rep.DeselectedRepositories
}

// LoadDeselectionSet reads the persisted deselection list as a lookup set.
func LoadDeselectionSet(baseResultsDir string) DeselectionSet {
	return DeselectionKeys(LoadDeselectedRepos(baseResultsDir))
}

// ClearDeselectedRepos removes any persisted deselection so reports are rebuilt
// from the full scan. A fresh analysis run calls this: the previous run's selection
// refers to a repository set that no longer necessarily exists, and silently
// carrying it over would understate the new scan.
func ClearDeselectedRepos(baseResultsDir string) error {
	err := os.Remove(DeselectedReposPath(baseResultsDir))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
