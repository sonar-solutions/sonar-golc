package gogit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/SonarSource-Demos/sonar-golc/pkg/utils"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/protocol/packp/capability"
	"github.com/go-git/go-git/v5/plumbing/transport"
	//"github.com/go-git/go-git/v5/plumbing/transport/http"
)

// Getrepos shallow-clones a single branch of src into a fresh temp directory under
// workDir and returns the clone path.
//
// A non-zero timeout bounds the clone via context: go-git's PlainClone has no
// internal deadline, so a half-open TCP connection to the git server (VPN drop,
// proxy, server under load) would otherwise hang the cloning goroutine forever and
// stall the whole analysis. A timeout of 0 disables the deadline (legacy behavior).
//
// Clone failures are returned to the caller (and the partial clone is removed)
// instead of being swallowed: the previous code logged the error but still returned
// a valid path with a nil error, so callers treated a failed clone as an
// empty-but-successful repository.
func Getrepos(src, branch, token, workDir string, timeout time.Duration) (string, error) {

	loggers := utils.SharedLogger()
	suffix, err := randomSuffix()
	if err != nil {
		return "", err
	}

	baseDir, err := utils.ResolveWorkDir(workDir)
	if err != nil {
		return "", err
	}

	dst := filepath.Join(baseDir, fmt.Sprintf("gcloc-extract-%s", suffix))
	// Track the destination so it is swept on os.Exit() paths that skip deferred
	// cleanup (issue #81). Registered before cloning so a partial clone is covered too.
	utils.RegisterTempClone(dst)
	log.SetOutput(os.Stderr)

	transport.UnsupportedCapabilities = []capability.Capability{
		capability.ThinPack,
	}

	// Bound the clone with a context deadline when a timeout is configured.
	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	_, err = git.PlainCloneContext(ctx, dst, false, &git.CloneOptions{
		URL: src,

		ReferenceName: plumbing.NewBranchReferenceName(branch),
		//ReferenceName: plumbing.ReferenceName(branch),

		SingleBranch: true,
		Depth:        1,
	})

	if err != nil {
		re := regexp.MustCompile(`(https?:\/\/)[^@]+(@)`)
		maskedSrc := re.ReplaceAllString(src, "${1}*****${2}")

		// A partial clone may exist on disk; remove it and stop tracking it so a
		// failed clone does not leak into the work directory (issue #81).
		_ = os.RemoveAll(dst)
		utils.UnregisterTempClone(dst)

		// A deadline becomes an explicit timeout error so operators know to tune
		// CloneTimeout or exclude the offending repository.
		if errors.Is(err, context.DeadlineExceeded) {
			loggers.Errorf("\r\t\t\t\t❌ gogit.Getrepos: clone of branch %s timed out after %s -- Source: %s", branch, timeout, maskedSrc)
			return "", fmt.Errorf("clone timed out after %s", timeout)
		}
		loggers.Errorf("\r\t\t\t\t❌ Stack: gogit.Getrepos Git Branch %s - %s-- Source: %s -", branch, err, maskedSrc)
		return "", fmt.Errorf("clone failed: %w", err)
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
