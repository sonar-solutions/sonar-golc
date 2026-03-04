package utils

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/fatih/color"
	"github.com/sirupsen/logrus"
)

// Log path constants: compliant with README "Execution Log" (log file at <GoLCHome/Logs>/Logs.log).
// LogDir is the directory name; LogFileName is the file name; DefaultLogPath is the full relative path.
const (
	LogDir      = "Logs"
	LogFileName = "Logs.log"
)

var DefaultLogPath = filepath.Join(LogDir, LogFileName)

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

func NewLogger() *logrus.Logger {
	logger := logrus.New()
	logger.SetFormatter(&CustomFormatter{})

	logger.SetLevel(logrus.DebugLevel)

	logFile, err := os.OpenFile(DefaultLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		// Fallback to stdout only when log file cannot be created (e.g. read-only fs, Docker)
		logger.SetOutput(os.Stdout)
		return logger
	}

	logger.SetOutput(io.MultiWriter(os.Stdout, logFile))
	return logger
}
