package logger

import (
	"fmt"
	"os"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/muesli/termenv"
)

// LogLevel defines the severity of a log entry.
type LogLevel string

const (
	LevelDebug LogLevel = "debug"
	LevelInfo  LogLevel = "info"
	LevelError LogLevel = "error"
)

var (
	isJSON    bool
	isPlain   bool
	appLogger = log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: true,
	})
)

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

	content := fmt.Sprintf("%s\n%s", titleStyle.Render("✔  "+title), message)
	fmt.Fprintln(os.Stdout, boxStyle.Render(content))
}

// ErrorBanner prints a beautiful error box to os.Stderr.
func ErrorBanner(title, message string) {
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

	content := fmt.Sprintf("%s\n%s", titleStyle.Render("✘  "+title), message)
	fmt.Fprintln(os.Stderr, boxStyle.Render(content))
}

// Log emits a log line with a specific level and format arguments.
func Log(level LogLevel, format string, v ...interface{}) {
	switch level {
	case LevelDebug:
		appLogger.Debugf(format, v...)
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

// Debug logs a debug-level message.
func Debug(format string, v ...interface{}) {
	appLogger.Debugf(format, v...)
}

// Error logs an error message.
func Error(format string, v ...interface{}) {
	appLogger.Errorf(format, v...)
}
