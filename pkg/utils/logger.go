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

var (
	globalLevel     = logrus.InfoLevel
	globalLevelMu   sync.RWMutex
	sharedLogger    *logrus.Logger
	sharedLoggerOnce sync.Once
)

// SetGlobalLevel sets the log level used by all loggers returned from NewLogger().
// Call this early (e.g. from main after loading config and parsing CLI) so that
// every package respects the same level.
func SetGlobalLevel(level logrus.Level) {
	globalLevelMu.Lock()
	defer globalLevelMu.Unlock()
	globalLevel = level
}

// GetGlobalLevel returns the current global log level.
func GetGlobalLevel() logrus.Level {
	globalLevelMu.RLock()
	defer globalLevelMu.RUnlock()
	return globalLevel
}

type CustomFormatter struct{}

// Format formate l'enregistrement log
func (f *CustomFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	// Définir la couleur en fonction du niveau de log
	switch entry.Level {
	case logrus.DebugLevel:
		entry.Message = color.HiBlueString(entry.Message)
	case logrus.InfoLevel:
		entry.Message = color.WhiteString(entry.Message)
	case logrus.WarnLevel:
		entry.Message = color.YellowString(entry.Message)
	case logrus.ErrorLevel, logrus.FatalLevel, logrus.PanicLevel:
		entry.Message = color.RedString(entry.Message)
	}

	// Formater le temps
	timestamp := entry.Time.Format("2006-01-02 15:04:05")

	// Construire le message final
	msg := fmt.Sprintf("[%s] %s %s\n", timestamp, strings.ToUpper(entry.Level.String()), entry.Message)
	return []byte(msg), nil
}

// NewLogger returns a shared logger that writes to stdout and Logs/Logs.log.
// The log file is opened once per process to avoid file descriptor leaks when
// NewLogger() is called in hot paths (e.g. per-repository analysis).
func NewLogger() *logrus.Logger {
	sharedLoggerOnce.Do(func() {
		logger := logrus.New()
		logger.SetFormatter(&CustomFormatter{})

		globalLevelMu.RLock()
		level := globalLevel
		globalLevelMu.RUnlock()
		logger.SetLevel(level)

		logFile, err := os.OpenFile("Logs/Logs.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err != nil {
			// Fallback to stdout only when log file cannot be created (e.g. read-only fs, Docker)
			logger.SetOutput(os.Stdout)
			sharedLogger = logger
			return
		}

		logger.SetOutput(io.MultiWriter(os.Stdout, logFile))
		sharedLogger = logger
	})
	return sharedLogger
}
