package analyzer

import (
	"io/fs"
	"path/filepath"
	"strings"
)

// IgnoreFunc returns true if the given path (absolute) should be excluded from analysis.
// It is used for .gitignore-style exclusion when scanning local directories.
type IgnoreFunc func(absolutePath string, isDir bool) bool

type Analyzer struct {
	SupportedExtensions map[string]string
	path                string
	excludePaths        []string
	excludeExtensions   map[string]bool
	includeExtensions   map[string]bool
	ignoreFunc          IgnoreFunc
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

// SetIgnoreFunc sets the optional function used to skip ignored paths (e.g. from .gitignore).
// When set, MatchingFiles() will skip files and directories for which it returns true.
func (a *Analyzer) SetIgnoreFunc(f IgnoreFunc) {
	a.ignoreFunc = f
}

func (a *Analyzer) MatchingFiles() ([]FileMetadata, error) {
	var files []FileMetadata

	err := filepath.Walk(a.path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if a.ignoreFunc != nil {
			if a.ignoreFunc(path, info.IsDir()) {
				if info.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}

		if info.IsDir() {
			return nil
		}

		fileExtension := a.getFileExtension(path)
		if a.canAdd(path, fileExtension) {
			fm := FileMetadata{
				FilePath:  path,
				Extension: fileExtension,
				Language:  a.SupportedExtensions[fileExtension],
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

	_, ok := a.SupportedExtensions[extension]
	return ok
}
