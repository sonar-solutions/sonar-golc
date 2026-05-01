package utils

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetLogoPath_Missing(t *testing.T) {
	orig, _ := os.Getwd()
	tmp, err := os.MkdirTemp("", "logo_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(orig)
		_ = os.RemoveAll(tmp)
	}()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	if got := GetLogoPath(); got != "" {
		t.Errorf("expected empty string when logo is absent, got %q", got)
	}
}

func TestGetLogoPath_PresentInCWD(t *testing.T) {
	orig, _ := os.Getwd()
	tmp, err := os.MkdirTemp("", "logo_test_*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = os.Chdir(orig)
		_ = os.RemoveAll(tmp)
	}()
	if err := os.Chdir(tmp); err != nil {
		t.Fatal(err)
	}

	if err := os.MkdirAll(filepath.Join(tmp, "imgs"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "imgs", "Logob.png"), []byte("fake"), 0644); err != nil {
		t.Fatal(err)
	}

	got := GetLogoPath()
	if got == "" {
		t.Error("expected a non-empty path when logo exists in CWD")
	}
}
