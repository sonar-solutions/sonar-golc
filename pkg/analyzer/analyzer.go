package analyzer

import (
	"io/fs"
	"path/filepath"
	"strings"
)

type Analyzer struct {
	SupportedExtensions   map[string]string
	path                  string
	excludePaths          []string
	excludeExtensions     map[string]bool
	includeExtensions     map[string]bool
	excludeFolderKeywords []string
	excludeFilePatterns   []string
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
	excludeFolderKeywords []string,
	excludeFilePatterns []string,
) *Analyzer {
	return &Analyzer{
		SupportedExtensions:   extensions,
		path:                  path,
		excludePaths:          excludePaths,
		excludeExtensions:     excludeExtensions,
		includeExtensions:     includeExtensions,
		excludeFolderKeywords: excludeFolderKeywords,
		excludeFilePatterns:   excludeFilePatterns,
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

		fileExtension := a.getFileExtension(path)
		if a.canAdd(path, fileExtension) {
			fm := FileMetadata{
				FilePath:  path,
				Extension: fileExtension,
				// YAML and JSON are refined to their IaC dialect where the content says
				// so, because SonarQube counts those by default and plain YAML/JSON not
				// at all. Any other language is returned unchanged, unopened.
				Language: RefineLanguage(path, a.SupportedExtensions[fileExtension]),
			}
			files = append(files, fm)
		}

		return nil
	})

	return files, err
}

func (a *Analyzer) getFileExtension(path string) string {
	extension := filepath.Ext(path)

	if extension == "" {
		extension = filepath.Base(path)
	}

	return extension
}

func (a *Analyzer) canAdd(path string, extension string) bool {
	// Exact prefix-based path exclusion (legacy behaviour, still supported)
	for _, pathToExclude := range a.excludePaths {
		if strings.HasPrefix(path, pathToExclude) {
			return false
		}
	}

	// Folder keyword exclusion: delimiter-aware word match on every folder segment at any depth
	if len(a.excludeFolderKeywords) > 0 {
		rel := strings.TrimPrefix(path, a.path+string(filepath.Separator))
		dir := filepath.Dir(rel)
		if dir != "." {
			for _, segment := range strings.Split(dir, string(filepath.Separator)) {
				for _, kw := range a.excludeFolderKeywords {
					if folderSegmentContainsKeyword(segment, kw) {
						return false
					}
				}
			}
		}
	}

	// File name pattern exclusion: standard * wildcard matched against the base name only
	if len(a.excludeFilePatterns) > 0 {
		base := filepath.Base(path)
		for _, pattern := range a.excludeFilePatterns {
			if matched, _ := filepath.Match(pattern, base); matched {
				return false
			}
		}
	}

	// Resolve the extension rules first. They are pure string work, whereas the minified
	// check below opens and samples the file, so a .js the caller has already excluded by
	// extension should never be read at all.
	if len(a.includeExtensions) > 0 {
		if _, ok := a.includeExtensions[a.getFileExtension(path)]; !ok {
			return false
		}
	} else if _, ok := a.excludeExtensions[a.getFileExtension(path)]; ok {
		return false
	} else if _, ok := a.SupportedExtensions[extension]; !ok {
		return false
	}

	// Minified JavaScript and CSS never reach SonarQube's ncloc, so counting them would
	// over-estimate - a single committed bundle can be tens of thousands of lines. The
	// test is SonarJS's own, name and average line length both; see minified.go.
	return !looksMinified(path)
}

// folderSegmentContainsKeyword returns true if kw is a whole word within segment.
// Words are delimited by -, _ and . so "test" matches "integration-test" and
// "test_helpers" but not "protest" or "latest".
func folderSegmentContainsKeyword(segment, kw string) bool {
	kw = strings.ToLower(kw)
	segment = strings.ToLower(segment)
	if segment == kw {
		return true
	}
	for _, token := range strings.FieldsFunc(segment, func(r rune) bool {
		return r == '-' || r == '_' || r == '.'
	}) {
		if token == kw {
			return true
		}
	}
	return false
}
