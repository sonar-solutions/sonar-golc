package scanner

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// Redirected to a file, the bar must draw nothing. Every repaint is kept in a file, so on a
// large scan the bar becomes the bulk of the output and buries the log.
func TestProgressWriterDiscardsWhenRedirectedToAFile(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if got := progressWriter(f); got != io.Discard {
		t.Errorf("writer = %v, want io.Discard: a redirected stream would be flooded with repaints", got)
	}
}

// A pipe is what a shell pipeline and most CI runners give the process, and it is the case
// that has to be right - it is not a character device, so the bar must be discarded.
func TestProgressWriterDiscardsWhenPiped(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()

	if got := progressWriter(w); got != io.Discard {
		t.Errorf("writer = %v, want io.Discard for a pipe", got)
	}
}

// On a terminal the bar is wanted, and it must go to the very stream that was tested. A
// character device is the stand-in for a terminal here; /dev/null is one, which is why
// isTerminal accepts it.
func TestProgressWriterReturnsTheStreamItWasGivenOnATerminal(t *testing.T) {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()

	if !isTerminal(devNull) {
		t.Skip("this platform does not report " + os.DevNull + " as a character device")
	}

	got := progressWriter(devNull)
	if got != io.Writer(devNull) {
		t.Errorf("writer = %v, want the same stream that was tested; guarding one stream and "+
			"writing to another is how the bar escapes the check", got)
	}
}

// The end-to-end guarantee: with stdout redirected, a real bar built by createProgressbar and
// driven to completion must leave the destination empty.
//
// This is the test that would catch the bar moving to a stream nothing guards - the library's
// Default constructors write to stderr, so a change of constructor or an upgrade could do
// exactly that while progressWriter still looks correct.
func TestCreateProgressbarWritesNothingWhenStdoutIsRedirected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stdout.log")
	redirected, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}

	realStdout, realStderr := os.Stdout, os.Stderr
	os.Stdout = redirected
	// Capture stderr too: "nothing was written" has to mean nothing anywhere, otherwise a bar
	// that moved to stderr would pass a stdout-only assertion.
	stderrPath := filepath.Join(t.TempDir(), "stderr.log")
	redirectedErr, err := os.Create(stderrPath)
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = redirectedErr
	t.Cleanup(func() {
		os.Stdout, os.Stderr = realStdout, realStderr
		redirected.Close()
		redirectedErr.Close()
	})

	sc := NewScanner(nil)
	bar := sc.createProgressbar(10)
	for i := 0; i < 10; i++ {
		if err := bar.Add(1); err != nil {
			t.Fatalf("driving the bar failed: %v", err)
		}
	}

	for name, p := range map[string]string{"stdout": path, "stderr": stderrPath} {
		written, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if len(written) != 0 {
			t.Errorf("%d bytes reached the redirected %s; the bar is not guarded on that "+
				"stream:\n%q", len(written), name, written)
		}
	}
}
