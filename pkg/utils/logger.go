package utils

import (
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

// sharedLog is the process-wide singleton, reset once per analysis run.
var (
	sharedLog   *logrus.Logger
	sharedLogMu sync.RWMutex
)

// ResetSharedLogger creates a fresh singleton logger, opening new file handles.
// Call this once at the start of each analysis run, after Logs/ has been created.
func ResetSharedLogger() {
	sharedLogMu.Lock()
	defer sharedLogMu.Unlock()
	sharedLog = NewLogger()
}

// SharedLogger returns the process-wide singleton, initialising it on first use.
// All platform packages should use this instead of NewLogger() to avoid FD leaks.
func SharedLogger() *logrus.Logger {
	sharedLogMu.RLock()
	if sharedLog != nil {
		l := sharedLog
		sharedLogMu.RUnlock()
		return l
	}
	sharedLogMu.RUnlock()

	sharedLogMu.Lock()
	defer sharedLogMu.Unlock()
	if sharedLog == nil {
		sharedLog = NewLogger()
	}
	return sharedLog
}

// CustomFormatter writes coloured log lines to terminal/SSE output.
type CustomFormatter struct{}

func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	msg := entry.Message
	switch entry.Level {
	case logrus.DebugLevel:
		msg = color.HiBlueString(msg)
	case logrus.InfoLevel:
		msg = color.WhiteString(msg)
	case logrus.WarnLevel:
		msg = color.YellowString(msg)
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		msg = color.RedString(msg)
	}
	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s %s\n", timestamp, strings.ToUpper(entry.Level.String()), msg)
	return []byte(line), nil
}

// plainFormatter writes plain (no ANSI colour) log lines — used for debug.log.
type plainFormatter struct{}

func (f *plainFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	line := fmt.Sprintf("[%s] %s %s\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message)
	return []byte(line), nil
}

// writerHook routes log entries matching the given levels to a writer.
type writerHook struct {
	writer    io.Writer
	formatter logrus.Formatter
	levels    []logrus.Level
}

func (h *writerHook) Levels() []logrus.Level { return h.levels }

func (h *writerHook) Fire(entry *logrus.Entry) error {
	b, err := h.formatter.Format(entry)
	if err != nil {
		return err
	}
	_, err = h.writer.Write(b)
	return err
}

var infoAndAbove = []logrus.Level{
	logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel,
	logrus.WarnLevel, logrus.InfoLevel,
}

var allLevels = []logrus.Level{
	logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel,
	logrus.WarnLevel, logrus.InfoLevel, logrus.DebugLevel, logrus.TraceLevel,
}

// NewLogger returns a logger that:
//   - writes Info+ (coloured) to stdout and Logs/Logs.log  — visible in the UI SSE stream
//   - writes Debug+ (plain text) to Logs/debug.log         — downloadable from the UI
//
// Each call opens two file handles (Logs.log + debug.log) that are only closed
// when the GC finalizes the returned logger, so it is NOT safe to call per-repo
// on hot paths. Production code MUST use SharedLogger() instead; NewLogger is
// intended for test fixtures that want an isolated logger instance.
func NewLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetOutput(io.Discard) // all output goes through hooks

	coloured := &CustomFormatter{}
	plain := &plainFormatter{}

	// Info+ → stdout (+ Logs.log when available)
	var mainOut io.Writer = os.Stdout
	if logFile, err := os.OpenFile("Logs/Logs.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
		mainOut = io.MultiWriter(os.Stdout, logFile)
	}
	logger.AddHook(&writerHook{writer: mainOut, formatter: coloured, levels: infoAndAbove})

	// Debug+ → Logs/debug.log (plain, all levels)
	if debugFile, err := os.OpenFile("Logs/debug.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666); err == nil {
		logger.AddHook(&writerHook{writer: debugFile, formatter: plain, levels: allLevels})
	}

	return logger
}
