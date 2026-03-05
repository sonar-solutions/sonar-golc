package analyzer

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"
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
	log := utils.NewLogger()

	err := filepath.Walk(a.path, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		fileExtension := a.getFileExtension(path)
		ok, excludeReason := a.canAddWithReason(path, fileExtension)
		if ok {
			fm := FileMetadata{
				FilePath:  path,
				Extension: fileExtension,
				Language:  a.SupportedExtensions[fileExtension],
			}
			files = append(files, fm)
			log.Debugf("file included: path=%s extension=%s language=%s", path, fileExtension, fm.Language)
		} else {
			log.Debugf("file excluded: path=%s reason=%s", path, excludeReason)
		}

		return nil
	})

	log.Debugf("matching files: %d found under %s", len(files), a.path)
	return files, err
}

func (a *Analyzer) getFileExtension(path string) string {
	extension := filepath.Ext(path)

	if extension == "" {
		extension = filepath.Base(path)
	}

	return extension
}

// canAddWithReason returns whether the file should be included and, if not, a short reason for troubleshooting.
func (a *Analyzer) canAddWithReason(path string, extension string) (bool, string) {
	for _, pathToExclude := range a.excludePaths {
		if strings.HasPrefix(path, pathToExclude) {
			return false, "excluded path"
		}
	}

	if len(a.includeExtensions) > 0 {
		_, ok := a.includeExtensions[a.getFileExtension(path)]
		if !ok {
			return false, "not in include list"
		}
		return true, ""
	}

	if _, ok := a.excludeExtensions[a.getFileExtension(path)]; ok {
		return false, "excluded extension"
	}

	_, ok := a.SupportedExtensions[extension]
	if !ok {
		return false, "unsupported extension"
	}
	return true, ""
}
