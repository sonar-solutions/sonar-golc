package utils

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func names(dirs []FileModeDir) []string {
	var out []string
	for _, d := range dirs {
		out = append(out, d.Name)
	}
	return out
}

// A directory whose name is unique keeps it, so existing reports are named as before.
func TestFileModeDirsKeepsUniqueNames(t *testing.T) {
	got := FileModeDirs([]string{"/work/frontend", "/work/backend", "/srv/tools"})
	want := []string{"frontend", "backend", "tools"}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("names = %v, want %v", names(got), want)
	}
	if got[0].Path != "/work/frontend" {
		t.Errorf("path = %q, want the path as given", got[0].Path)
	}
}

// Directories that share a name take parent elements until they differ. Only the ones
// that clash grow; the rest keep their short name.
func TestFileModeDirsTellsSharedNamesApart(t *testing.T) {
	got := FileModeDirs([]string{
		"/scan/golc-step0/nb",
		"/scan/golc-step0b/nb",
		"/scan/golc-step0/docs",
		// Same parent name too, so these need one more element.
		"/x/team/app",
		"/y/team/app",
	})
	want := []string{"golc-step0_nb", "golc-step0b_nb", "docs", "x_team_app", "y_team_app"}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("names = %v, want %v", names(got), want)
	}
}

// Listing a directory twice - with a trailing slash, a "." segment, or once relative and
// once absolute - must scan it once, or distinct names would count it twice.
func TestFileModeDirsScansARepeatedDirectoryOnce(t *testing.T) {
	abs, err := filepath.Abs("repo")
	if err != nil {
		t.Fatal(err)
	}
	got := FileModeDirs([]string{"repo", abs, abs + string(filepath.Separator), filepath.Join(abs, "."), "other"})
	want := []string{"repo", "other"}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("names = %v, want %v", names(got), want)
	}
}

// A directory that runs out of parents keeps the short name, and the one that still has
// parents takes them.
func TestFileModeDirsWhenOneRunsOutOfParents(t *testing.T) {
	got := FileModeDirs([]string{"/app", "/srv/app"})
	want := []string{"app", "srv_app"}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("names = %v, want %v", names(got), want)
	}
}

// Whatever lengthening cannot separate gets a numeric suffix, so names are always unique.
func TestSettleTiesMakesEveryNameUnique(t *testing.T) {
	dirs := []FileModeDir{{Name: "app"}, {Name: "app"}, {Name: "app-2"}, {Name: "app"}}
	settleTies(dirs)
	want := []string{"app", "app-2", "app-2-2", "app-3"}
	if !reflect.DeepEqual(names(dirs), want) {
		t.Errorf("names = %v, want %v", names(dirs), want)
	}
}

func TestFileModeDirsEmpty(t *testing.T) {
	if got := FileModeDirs(nil); len(got) != 0 {
		t.Errorf("got %v, want nothing", got)
	}
}

// A drive letter must not carry its colon into a name, because the name becomes a file
// name and Windows does not allow ":" in one.
func TestPathElementsDropVolumeColon(t *testing.T) {
	got := pathElements("C:/srv/app")
	want := []string{"C", "srv", "app"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("pathElements = %v, want %v", got, want)
	}
}

func TestFileModeDirsNamesAreValidFileNames(t *testing.T) {
	// The drive letter sits beneath / here so the test runs on every OS; on Windows
	// C:\app and D:\app reach the same naming step with the same elements.
	got := FileModeDirs([]string{"/C:/app", "/D:/app"})
	want := []string{"C_app", "D_app"}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("names = %v, want %v", names(got), want)
	}
	for _, d := range got {
		if strings.ContainsAny(d.Name, `:\/`) {
			t.Errorf("name %q contains a character that is not valid in a file name", d.Name)
		}
	}
}
