package analyzer

import (
	"io/fs"
	"path/filepath"
	"strings"
)

type Analyzer struct {
	SupportedExtensions map[string]string
	path                string
	excludePaths        []string
	excludeExtensions   map[string]bool
	includeExtensions   map[string]bool
}

type FileMetadata struct {
	FilePath  string
	Extension string
	Language  string
}

func NewAnalyzer(
	path string,
	excludePaths []string,
	excludeExtensions map[string]bool,
	includeExtensions map[string]bool,
	extensions map[string]string,
) *Analyzer {
	return &Analyzer{
		SupportedExtensions: extensions,
		path:                path,
		excludePaths:        excludePaths,
		excludeExtensions:   excludeExtensions,
		includeExtensions:   includeExtensions,
	}
}

func (a *Analyzer) MatchingFiles() ([]FileMetadata, error) {
	var files []FileMetadata

	err := filepath.Walk(a.path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		languageKey := a.getLanguageKey(path)
		if a.canAdd(path, languageKey) {
			fm := FileMetadata{
				FilePath:  path,
				Extension: languageKey,
				Language:  a.SupportedExtensions[languageKey],
			}
			files = append(files, fm)
		}

		return nil
	})

	return files, err
}

// getLanguageKey returns the key for SupportedExtensions lookup.
// It checks path patterns first (e.g., .github/workflows/*.yml), then specific filenames, then extension.
func (a *Analyzer) getLanguageKey(path string) string {
	base := filepath.Base(path)
	ext := filepath.Ext(path)

	// Check path patterns: path:.github/workflows/.yml, path:charts/.yaml, etc.
	if ext == ".yml" || ext == ".yaml" {
		if strings.Contains(path, ".github/workflows/") {
			return "path:.github/workflows/" + ext
		}
		if strings.Contains(path, string(filepath.Separator)+"charts"+string(filepath.Separator)) {
			return "path:charts/" + ext
		}
	}

	// Check specific filenames (basename match)
	if _, ok := a.SupportedExtensions[base]; ok {
		return base
	}

	// Fall back to extension
	if ext == "" {
		return base
	}
	return ext
}

func (a *Analyzer) getFileExtension(path string) string {
	extension := filepath.Ext(path)

	if extension == "" {
		extension = filepath.Base(path)
	}

	return extension
}

func (a *Analyzer) canAdd(path string, languageKey string) bool {
	for _, pathToExclude := range a.excludePaths {
		if strings.HasPrefix(path, pathToExclude) {
			return false
		}
	}

	if len(a.includeExtensions) > 0 {
		_, ok := a.includeExtensions[a.getFileExtension(path)]
		return ok
	}

	if _, ok := a.excludeExtensions[a.getFileExtension(path)]; ok {
		return false
	}

	_, ok := a.SupportedExtensions[languageKey]
	return ok
}
