// Package logging provides internal logging utilities for MCPCat.
// Logs are written to ~/mcpcat.log to avoid interfering with STDIO-based MCP servers.
package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
)

// Logger provides logging functionality for MCPCat
type Logger struct {
	file   *os.File
	logger *log.Logger
	mu     sync.Mutex
	debug  bool
}

var (
	defaultLogger *Logger
	loggerMu      sync.Mutex
	globalDebug   bool
	globalDebugMu sync.RWMutex
)

// SetGlobalDebug sets the global debug flag for all logger instances
func SetGlobalDebug(debug bool) {
	globalDebugMu.Lock()
	defer globalDebugMu.Unlock()
	globalDebug = debug

	loggerMu.Lock()
	defer loggerMu.Unlock()
	if defaultLogger != nil {
		defaultLogger.mu.Lock()
		defaultLogger.debug = debug
		defaultLogger.updateWriter()
		defaultLogger.mu.Unlock()
	}
}

// New returns the singleton logger, creating it on first call.
// After ResetForTesting, a fresh logger is created on the next call.
func New() *Logger {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if defaultLogger == nil {
		defaultLogger = newLogger()
	}
	return defaultLogger
}

// ResetForTesting closes the current logger and resets the singleton
// so the next New() call creates a fresh instance.
func ResetForTesting() {
	loggerMu.Lock()
	defer loggerMu.Unlock()
	if defaultLogger != nil {
		defaultLogger.Close()
	}
	defaultLogger = nil
}

func newLogger() *Logger {
	globalDebugMu.RLock()
	debug := globalDebug
	globalDebugMu.RUnlock()

	l := &Logger{
		debug: debug,
	}

	if debug {
		l.openLogFile()
	}

	var writer io.Writer
	if debug && l.file != nil {
		writer = l.file
	} else {
		writer = io.Discard
	}
	l.logger = log.New(writer, "[MCPCat] ", log.LstdFlags)

	return l
}

// openLogFile opens ~/mcpcat.log for appending. On failure the file field
// stays nil and all output silently goes to io.Discard, which avoids ever
// falling back to stderr and corrupting STDIO-based MCP transport.
func (l *Logger) openLogFile() {
	homeDir, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDir, "mcpcat.log")
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		l.file = nil
		return
	}
	l.file = file
}

func (l *Logger) updateWriter() {
	if l.debug {
		if l.file == nil {
			l.openLogFile()
		}
		if l.file != nil {
			l.logger.SetOutput(l.file)
			return
		}
	} else {
		if l.file != nil {
			l.file.Close()
			l.file = nil
		}
	}
	l.logger.SetOutput(io.Discard)
}

// Info logs an informational message
func (l *Logger) Info(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("INFO: %s", msg)
}

// Infof logs a formatted informational message
func (l *Logger) Infof(format string, args ...interface{}) {
	l.Info(fmt.Sprintf(format, args...))
}

// Warn logs a warning message
func (l *Logger) Warn(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("WARN: %s", msg)
}

// Warnf logs a formatted warning message
func (l *Logger) Warnf(format string, args ...interface{}) {
	l.Warn(fmt.Sprintf(format, args...))
}

// Error logs an error message
func (l *Logger) Error(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("ERROR: %s", msg)
}

// Errorf logs a formatted error message
func (l *Logger) Errorf(format string, args ...interface{}) {
	l.Error(fmt.Sprintf(format, args...))
}

// Debug logs a debug message
func (l *Logger) Debug(msg string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("DEBUG: %s", msg)
}

// Debugf logs a formatted debug message
func (l *Logger) Debugf(format string, args ...interface{}) {
	l.Debug(fmt.Sprintf(format, args...))
}

// Close closes the log file
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}
