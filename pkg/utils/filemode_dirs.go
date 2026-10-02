package utils

import (
	"fmt"
	"path/filepath"
	"strings"
)

// FileModeDir is one directory the File platform scans, and the name its results are
// reported under. That name is the stem of its result files (Result_<Name>.json) and the
// repository name on the results page, so no two directories may share it.
type FileModeDir struct {
	Path string
	Name string
}

// FileModeDirs assigns each directory a distinct report name.
//
// A directory is named after its last path element, as it always has been - unless that
// name is shared with another directory in the list, which happens readily: two
// checkouts that both contain an "app" folder, or "scan subdirectories" over two trees
// with the same layout. Sharing a name used to make the second directory's result files
// overwrite the first's, so its lines vanished from the totals without a word. Directories
// that share a name now take as many parent elements as it takes to tell them apart,
// joined with "_": /work/a/app and /work/b/app become a_app and b_app.
//
// The same directory listed twice is scanned once, since distinct names would otherwise
// count it twice.
func FileModeDirs(paths []string) []FileModeDir {
	dirs, elements := uniqueDirs(paths)
	lengthenSharedNames(dirs, elements)
	settleTies(dirs)

	return dirs
}

// uniqueDirs drops repeats of a directory, comparing absolute cleaned paths, and returns
// the path elements of each directory kept.
func uniqueDirs(paths []string) ([]FileModeDir, [][]string) {
	var dirs []FileModeDir
	var elements [][]string
	seen := map[string]bool{}

	for _, p := range paths {
		key := filepath.Clean(p)
		if abs, err := filepath.Abs(key); err == nil {
			key = abs
		}
		if seen[key] {
			continue
		}
		seen[key] = true

		dirs = append(dirs, FileModeDir{Path: p})
		elements = append(elements, pathElements(key))
	}

	return dirs, elements
}

// lengthenSharedNames names each directory after its last path element, then keeps adding
// parent elements to the names that are shared until they differ or run out of parents.
func lengthenSharedNames(dirs []FileModeDir, elements [][]string) {
	depth := make([]int, len(dirs))
	for i := range depth {
		depth[i] = 1
	}

	for {
		holders := map[string][]int{}
		for i := range dirs {
			dirs[i].Name = joinLastElements(elements[i], depth[i])
			holders[dirs[i].Name] = append(holders[dirs[i].Name], i)
		}

		grew := false
		for _, group := range holders {
			if len(group) < 2 {
				continue
			}
			for _, i := range group {
				if depth[i] < len(elements[i]) {
					depth[i]++
					grew = true
				}
			}
		}
		if !grew {
			return
		}
	}
}

// settleTies appends a numeric suffix to any name still shared - possible when a
// directory runs out of parents, or when a lengthened name such as a_app is also the
// name of another directory.
func settleTies(dirs []FileModeDir) {
	used := map[string]bool{}
	for i := range dirs {
		name := dirs[i].Name
		for n := 2; used[name]; n++ {
			name = fmt.Sprintf("%s-%d", dirs[i].Name, n)
		}
		used[name] = true
		dirs[i].Name = name
	}
}

// pathElements splits a cleaned absolute path into its non-empty elements. A volume
// name such as "C:" is kept as an element of its own.
func pathElements(path string) []string {
	var elements []string
	for _, e := range strings.Split(filepath.ToSlash(path), "/") {
		if e != "" {
			elements = append(elements, e)
		}
	}
	if len(elements) == 0 {
		return []string{filepath.Base(path)}
	}

	return elements
}

func joinLastElements(elements []string, n int) string {
	if n > len(elements) {
		n = len(elements)
	}

	return strings.Join(elements[len(elements)-n:], "_")
}
