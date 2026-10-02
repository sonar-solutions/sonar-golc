package goloc

import (
	"path/filepath"
	"testing"

	"github.com/SonarSource-Demos/sonar-golc/assets"
)

// The result files are named OutputName plus the directory's name, unless Name says
// otherwise - which the File platform relies on to keep two same-named directories from
// writing the same files.
func TestNewGClocReportName(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name string
		want string
	}{
		{"", "Result_" + filepath.Base(dir)},
		{"team_app", "Result_team_app"},
	}
	for _, tc := range tests {
		gc, err := NewGCloc(Params{Path: dir, Name: tc.name, OutputName: "Result_"}, assets.Languages)
		if err != nil {
			t.Fatalf("NewGCloc: %v", err)
		}
		if gc.Params.OutputName != tc.want {
			t.Errorf("Name %q: OutputName = %q, want %q", tc.name, gc.Params.OutputName, tc.want)
		}
	}
}
