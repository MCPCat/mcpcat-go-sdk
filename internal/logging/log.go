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
	defaultLogger     *Logger
	defaultLoggerOnce sync.Once
	globalDebug       bool
	globalDebugMu     sync.RWMutex
)

// SetGlobalDebug sets the global debug flag for all logger instances
func SetGlobalDebug(debug bool) {
	globalDebugMu.Lock()
	defer globalDebugMu.Unlock()
	globalDebug = debug

	// Update existing default logger if it exists
	if defaultLogger != nil {
		defaultLogger.mu.Lock()
		defaultLogger.debug = debug
		defaultLogger.updateWriter()
		defaultLogger.mu.Unlock()
	}
}

// New creates a new logger instance
func New() *Logger {
	defaultLoggerOnce.Do(func() {
		defaultLogger = newLogger()
	})
	return defaultLogger
}

func newLogger() *Logger {
	globalDebugMu.RLock()
	debug := globalDebug
	globalDebugMu.RUnlock()

	homeDir, _ := os.UserHomeDir()
	logPath := filepath.Join(homeDir, "mcpcat.log")

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		file = os.Stderr
	}

	logger := &Logger{
		file:  file,
		debug: debug,
	}

	// Set the writer based on debug flag
	var writer io.Writer
	if debug {
		writer = file
	} else {
		writer = io.Discard
	}
	logger.logger = log.New(writer, "[MCPCat] ", log.LstdFlags)

	return logger
}

// updateWriter updates the logger's writer based on the debug flag
func (l *Logger) updateWriter() {
	var writer io.Writer
	if l.debug {
		writer = l.file
	} else {
		writer = io.Discard
	}
	l.logger.SetOutput(writer)
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
	// TODO: Add debug level control via environment variable
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
	if l.file != nil && l.file != os.Stderr {
		return l.file.Close()
	}
	return nil
}
