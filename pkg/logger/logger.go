package logger

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/charmbracelet/x/term"
	"github.com/mattn/go-isatty"
	"github.com/muesli/termenv"
)

// LogLevel defines the severity of a log entry.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

var (
	isJSON    bool
	isPlain   bool
	appLogger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
	})
	logFileHandle *os.File
	logFilePath   string
	isTempLogFile bool
)

// TempDir returns the directory path for temporary files.
// It respects XDG_RUNTIME_DIR if set and pointing to an existing directory.
// Otherwise, it falls back to os.TempDir().
func TempDir() string {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		if fi, err := os.Stat(xdg); err == nil && fi.IsDir() {
			return xdg
		}
	}
	return os.TempDir()
}

// InitFileLogging configures streaming logs to a file.
// If filePath is empty, a temporary log file is created.
func InitFileLogging(filePath string) error {
	if logFileHandle != nil {
		_ = logFileHandle.Close()
	}

	var f *os.File
	var err error
	var isTemp bool

	if filePath != "" {
		f, err = os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
		if err != nil {
			return fmt.Errorf("failed to open log file %q: %w", filePath, err)
		}
		isTemp = false
	} else {
		f, err = os.CreateTemp(TempDir(), "aetherpak-*.log")
		if err != nil {
			return fmt.Errorf("failed to create temporary log file: %w", err)
		}
		filePath = f.Name()
		isTemp = true
	}

	logFileHandle = f
	logFilePath = filePath
	isTempLogFile = isTemp

	appLogger.SetOutput(io.MultiWriter(os.Stderr, f))
	return nil
}

// CloseLogFile closes the file logging stream and cleans up temp files if successful.
func CloseLogFile(hasError bool) {
	if logFileHandle == nil {
		return
	}

	_ = logFileHandle.Close()
	appLogger.SetOutput(os.Stderr)

	path := logFilePath
	isTemp := isTempLogFile

	logFileHandle = nil
	logFilePath = ""
	isTempLogFile = false

	if isTemp {
		if hasError {
			if isPlain {
				fmt.Fprintf(os.Stderr, "\nDetailed logs written to: %s\n", path)
			} else {
				style := lipgloss.NewStyle().Foreground(lipgloss.Color("86")).Bold(true)
				fmt.Fprintf(os.Stderr, "\nDetailed logs written to: %s\n", style.Render(path))
			}
		} else {
			_ = os.Remove(path)
		}
	}
}

// Init configures the logger parameters.
func Init(verbose, useJSON, usePlain bool) {
	isJSON = useJSON
	if os.Getenv("CI") != "" {
		usePlain = true
	}
	isPlain = usePlain
	if useJSON {
		appLogger.SetFormatter(log.JSONFormatter)
	} else {
		appLogger.SetFormatter(log.TextFormatter)
	}

	if verbose {
		appLogger.SetLevel(log.DebugLevel)
	} else {
		appLogger.SetLevel(log.InfoLevel)
	}

	if isPlain {
		appLogger.SetColorProfile(termenv.Ascii)
		lipgloss.SetColorProfile(termenv.Ascii)
	}
}

// IsPlain returns whether the logger is in plain text mode.
func IsPlain() bool {
	return isPlain
}

// SuccessBanner prints a beautiful success completion box to os.Stdout.
func SuccessBanner(title, message string) {
	if logFileHandle != nil {
		fmt.Fprintf(logFileHandle, "\n✔  %s\n%s\n\n", title, message)
	}

	if isJSON {
		appLogger.Infof("%s: %s", title, message)
		return
	}
	if isPlain {
		fmt.Fprintf(os.Stdout, "\n✔  %s\n%s\n\n", title, message)
		return
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("42"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("42")).
		Padding(1, 2).
		Margin(1, 0)

	width := 80
	if isatty.IsTerminal(os.Stdout.Fd()) {
		if w, _, err := term.GetSize(uintptr(os.Stdout.Fd())); err == nil && w > 15 {
			width = w
		}
	}
	if width > 100 {
		width = 100
	}
	boxStyle = boxStyle.Width(width - 8)

	content := fmt.Sprintf("%s\n%s", titleStyle.Render("✔  "+title), message)
	fmt.Fprintln(os.Stdout, boxStyle.Render(content))
}

// ErrorBanner prints a beautiful error box to os.Stderr.
func ErrorBanner(title, message string) {
	if logFileHandle != nil {
		fmt.Fprintf(logFileHandle, "\n✘  %s\n%s\n\n", title, message)
	}

	if isJSON {
		appLogger.Errorf("%s: %s", title, message)
		return
	}
	if isPlain {
		fmt.Fprintf(os.Stderr, "\n✘  %s\n%s\n\n", title, message)
		return
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("203"))

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("203")).
		Padding(1, 2).
		Margin(1, 0)

	width := 80
	if isatty.IsTerminal(os.Stderr.Fd()) {
		if w, _, err := term.GetSize(uintptr(os.Stderr.Fd())); err == nil && w > 15 {
			width = w
		}
	}
	if width > 100 {
		width = 100
	}
	boxStyle = boxStyle.Width(width - 8)

	content := fmt.Sprintf("%s\n%s", titleStyle.Render("✘  "+title), message)
	fmt.Fprintln(os.Stderr, boxStyle.Render(content))
}

// Log emits a log line with a specific level and format arguments.
func Log(level LogLevel, format string, v ...interface{}) {
	switch level {
	case LevelDebug:
		appLogger.Debugf(format, v...)
	case LevelWarn:
		appLogger.Warnf(format, v...)
	case LevelError:
		appLogger.Errorf(format, v...)
	default:
		appLogger.Infof(format, v...)
	}
}

// Info logs an informational message.
func Info(format string, v ...interface{}) {
	appLogger.Infof(format, v...)
}

// Warn logs a warning message.
func Warn(format string, v ...interface{}) {
	appLogger.Warnf(format, v...)
}

// Debug logs a debug-level message.
func Debug(format string, v ...interface{}) {
	appLogger.Debugf(format, v...)
}

// Error logs an error message.
func Error(format string, v ...interface{}) {
	appLogger.Errorf(format, v...)
}
