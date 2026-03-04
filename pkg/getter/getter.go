package getter

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/briandowns/spinner"
	getter "github.com/hashicorp/go-getter"
)

func extractLastString(url string) string {
	/*parts := strings.Split(url, "/")
	return parts[len(parts)-1]*/

	return filepath.Base(url)
}

// toFileURL converts an absolute local path to a file:// URL so that go-getter
// reliably recognizes it on all platforms. On Windows, paths like C:\repo are
// converted to file:///C:/repo.
func toFileURL(absPath string) string {
	if runtime.GOOS != "windows" {
		return absPath
	}
	// Windows: file:///C:/path or file:///C:/ (drive root)
	slash := filepath.ToSlash(absPath)
	if len(slash) >= 2 && slash[1] == ':' {
		return "file:///" + slash
	}
	return absPath
}

func Getter(src string) (string, error) {
	// On Windows, pass local absolute paths as file:// URLs so the getter accepts them.
	if runtime.GOOS == "windows" && len(src) >= 2 && src[1] == ':' && !strings.HasPrefix(src, "file://") {
		src = toFileURL(src)
	}
	RepoString := extractLastString(src)

	spinner := newSpinner(fmt.Sprintf("\r Extracting files from %s \n", RepoString))
	spinner.Color("green", "bold")
	messageF := ""
	spinner.FinalMSG = messageF
	spinner.Start()
	defer spinner.Stop()

	suffix, err := randomSuffix()
	if err != nil {
		return "", err
	}

	dst := filepath.Join(os.TempDir(), fmt.Sprintf("gcloc-extract-%s", suffix))
	pwd, err := os.Getwd()
	if err != nil {
		return "", err
	}

	client := &getter.Client{
		Src: src,
		Dst: dst,
		Pwd: pwd,
		//Mode: getter.ClientModeAny,
		Mode: getter.ClientModeDir,
	}

	if err := client.Get(); err != nil {
		return "", err
	}

	symLink, err := isSymLink(dst)
	if err != nil {
		return "", err
	}

	if symLink {
		origin, err := os.Readlink(dst)
		if err != nil {
			return "", err
		}

		return origin, nil
	}

	return dst, nil
}

func newSpinner(text string) *spinner.Spinner {
	return spinner.New(
		spinner.CharSets[35],
		100*time.Millisecond,
		spinner.WithSuffix(text),
	)
}

func randomSuffix() (string, error) {
	randBytes := make([]byte, 16)
	_, err := rand.Read(randBytes)
	if err != nil {
		return "", err
	}

	return hex.EncodeToString(randBytes), nil
}

func isSymLink(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return false, err
	}

	return info.Mode()&os.ModeSymlink != 0, nil
}
