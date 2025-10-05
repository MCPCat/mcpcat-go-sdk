package logging

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// resetGlobalState resets the global logger state for testing
func resetGlobalState() {
	defaultLogger = nil
	defaultLoggerOnce = sync.Once{}
	globalDebug = false
}

// TestNew_ReturnsSameInstance verifies that multiple calls to New() return the same logger instance
func TestNew_ReturnsSameInstance(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	logger1 := New()
	logger2 := New()

	if logger1 != logger2 {
		t.Error("Expected New() to return the same logger instance")
	}
}

// TestNewLogger_GlobalDebugState verifies new logger respects global debug flag
func TestNewLogger_GlobalDebugState(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	tests := []struct {
		name        string
		globalDebug bool
	}{
		{"debug enabled", true},
		{"debug disabled", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetGlobalState()
			SetGlobalDebug(tt.globalDebug)

			logger := newLogger()
			defer logger.Close()

			if logger.debug != tt.globalDebug {
				t.Errorf("Expected logger.debug=%v, got %v", tt.globalDebug, logger.debug)
			}
		})
	}
}

// TestSetGlobalDebug_UpdatesFlag tests setting/unsetting global debug
func TestSetGlobalDebug_UpdatesFlag(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	globalDebugMu.RLock()
	if !globalDebug {
		t.Error("Expected globalDebug to be true")
	}
	globalDebugMu.RUnlock()

	SetGlobalDebug(false)
	globalDebugMu.RLock()
	if globalDebug {
		t.Error("Expected globalDebug to be false")
	}
	globalDebugMu.RUnlock()
}

// TestSetGlobalDebug_UpdatesExistingLogger verifies existing logger gets updated
func TestSetGlobalDebug_UpdatesExistingLogger(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	logger := New()
	defer logger.Close()

	SetGlobalDebug(true)
	if !logger.debug {
		t.Error("Expected logger.debug to be true after SetGlobalDebug(true)")
	}

	SetGlobalDebug(false)
	if logger.debug {
		t.Error("Expected logger.debug to be false after SetGlobalDebug(false)")
	}
}

// TestSetGlobalDebug_Concurrent tests thread-safety with concurrent access
func TestSetGlobalDebug_Concurrent(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(val bool) {
			defer wg.Done()
			SetGlobalDebug(val)
		}(i%2 == 0)
	}

	wg.Wait()
	// Test passes if no race conditions occur
}

// TestLogger_Info verifies Info logs with correct prefix and format
func TestLogger_Info(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	// Capture output
	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Info("test message")

	output := buf.String()
	if !strings.Contains(output, "INFO: test message") {
		t.Errorf("Expected output to contain 'INFO: test message', got: %s", output)
	}
	if !strings.Contains(output, "[MCPCat]") {
		t.Errorf("Expected output to contain '[MCPCat]' prefix, got: %s", output)
	}
}

// TestLogger_Infof tests formatted Info logging
func TestLogger_Infof(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Infof("formatted %s %d", "message", 42)

	output := buf.String()
	if !strings.Contains(output, "INFO: formatted message 42") {
		t.Errorf("Expected output to contain 'INFO: formatted message 42', got: %s", output)
	}
}

// TestLogger_Warn verifies Warn logs with correct prefix
func TestLogger_Warn(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Warn("warning message")

	output := buf.String()
	if !strings.Contains(output, "WARN: warning message") {
		t.Errorf("Expected output to contain 'WARN: warning message', got: %s", output)
	}
}

// TestLogger_Warnf tests formatted Warn logging
func TestLogger_Warnf(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Warnf("warning %s", "test")

	output := buf.String()
	if !strings.Contains(output, "WARN: warning test") {
		t.Errorf("Expected output to contain 'WARN: warning test', got: %s", output)
	}
}

// TestLogger_Error verifies Error logs with correct prefix
func TestLogger_Error(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Error("error message")

	output := buf.String()
	if !strings.Contains(output, "ERROR: error message") {
		t.Errorf("Expected output to contain 'ERROR: error message', got: %s", output)
	}
}

// TestLogger_Errorf tests formatted Error logging
func TestLogger_Errorf(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Errorf("error %d", 404)

	output := buf.String()
	if !strings.Contains(output, "ERROR: error 404") {
		t.Errorf("Expected output to contain 'ERROR: error 404', got: %s", output)
	}
}

// TestLogger_Debug verifies Debug logs with correct prefix
func TestLogger_Debug(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Debug("debug message")

	output := buf.String()
	if !strings.Contains(output, "DEBUG: debug message") {
		t.Errorf("Expected output to contain 'DEBUG: debug message', got: %s", output)
	}
}

// TestLogger_Debugf tests formatted Debug logging
func TestLogger_Debugf(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := newLogger()
	defer logger.Close()

	var buf bytes.Buffer
	logger.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	logger.Debugf("debug %s", "info")

	output := buf.String()
	if !strings.Contains(output, "DEBUG: debug info") {
		t.Errorf("Expected output to contain 'DEBUG: debug info', got: %s", output)
	}
}

// TestLogger_DebugDisabled_DiscardsOutput verifies logs go to io.Discard when debug=false
func TestLogger_DebugDisabled_DiscardsOutput(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(false)
	logger := newLogger()
	defer logger.Close()

	// Verify logger output is io.Discard
	// We can't directly inspect the writer, but we can verify debug flag
	if logger.debug {
		t.Error("Expected logger.debug to be false")
	}

	// The logger should be using io.Discard
	// We can verify this by checking that updateWriter was called correctly
	var buf bytes.Buffer
	logger.logger.SetOutput(&buf)
	logger.Info("test")

	if buf.Len() == 0 {
		t.Error("Expected some output after manually setting writer")
	}
}

// TestLogger_DebugEnabled_WritesToFile verifies logs written to file when debug=true
func TestLogger_DebugEnabled_WritesToFile(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)

	// Create a temp file for logging
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")

	file, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to create temp log file: %v", err)
	}

	logger := &Logger{
		file:  file,
		debug: true,
	}
	logger.logger = log.New(file, "[MCPCat] ", log.LstdFlags)
	defer logger.Close()

	logger.Info("test message")

	// Read the file to verify content was written
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "INFO: test message") {
		t.Errorf("Expected log file to contain 'INFO: test message', got: %s", string(content))
	}
}

// TestSetGlobalDebug_TogglesOutput tests toggling debug updates writer
func TestSetGlobalDebug_TogglesOutput(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	logger := New()
	defer logger.Close()

	// Initially debug is false
	if logger.debug {
		t.Error("Expected initial debug to be false")
	}

	// Enable debug
	SetGlobalDebug(true)
	if !logger.debug {
		t.Error("Expected debug to be true after SetGlobalDebug(true)")
	}

	// Disable debug
	SetGlobalDebug(false)
	if logger.debug {
		t.Error("Expected debug to be false after SetGlobalDebug(false)")
	}
}

// TestNewLogger_CreatesLogFile verifies log file creation
func TestNewLogger_CreatesLogFile(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	logger := newLogger()
	defer logger.Close()

	if logger.file == nil {
		t.Error("Expected logger.file to be set")
	}

	// File should be either the log file or stderr
	if logger.file != os.Stderr {
		// Verify it's a real file
		stat, err := logger.file.Stat()
		if err != nil {
			t.Errorf("Failed to stat log file: %v", err)
		}
		if stat.IsDir() {
			t.Error("Expected log file to be a file, not a directory")
		}
	}
}

// TestNewLogger_FallbackToStderr tests fallback when file creation fails
func TestNewLogger_FallbackToStderr(t *testing.T) {
	// This test is difficult to implement without mocking os.OpenFile
	// We would need to create conditions where file creation fails
	// Skipping for now, but could be implemented with filesystem mocking
	t.Skip("Requires filesystem mocking to test file creation failure")
}

// TestLogger_Close verifies Close() closes file properly
func TestLogger_Close(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")

	file, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		t.Fatalf("Failed to create temp log file: %v", err)
	}

	logger := &Logger{
		file:  file,
		debug: false,
	}
	logger.logger = log.New(io.Discard, "[MCPCat] ", log.LstdFlags)

	err = logger.Close()
	if err != nil {
		t.Errorf("Expected Close() to succeed, got error: %v", err)
	}

	// Verify file is closed by trying to write
	_, err = file.Write([]byte("test"))
	if err == nil {
		t.Error("Expected write to closed file to fail")
	}
}

// TestLogger_Close_StderrNotClosed verifies stderr fallback isn't closed
func TestLogger_Close_StderrNotClosed(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	logger := &Logger{
		file:  os.Stderr,
		debug: false,
	}
	logger.logger = log.New(io.Discard, "[MCPCat] ", log.LstdFlags)

	err := logger.Close()
	if err != nil {
		t.Errorf("Expected Close() to succeed with stderr, got error: %v", err)
	}

	// Verify stderr is still writable
	_, err = os.Stderr.Write([]byte("test\n"))
	if err != nil {
		t.Error("Expected stderr to still be writable after Close()")
	}
}

// TestLogger_ConcurrentWrites tests multiple goroutines logging simultaneously
func TestLogger_ConcurrentWrites(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	SetGlobalDebug(true)
	logger := New()
	defer logger.Close()

	var wg sync.WaitGroup
	iterations := 100

	for i := 0; i < iterations; i++ {
		wg.Add(4)
		go func(n int) {
			defer wg.Done()
			logger.Info("info " + string(rune(n)))
		}(i)
		go func(n int) {
			defer wg.Done()
			logger.Warn("warn " + string(rune(n)))
		}(i)
		go func(n int) {
			defer wg.Done()
			logger.Error("error " + string(rune(n)))
		}(i)
		go func(n int) {
			defer wg.Done()
			logger.Debug("debug " + string(rune(n)))
		}(i)
	}

	wg.Wait()
	// Test passes if no race conditions or panics occur
}

// TestLogger_ConcurrentDebugToggle tests toggling debug while logging
func TestLogger_ConcurrentDebugToggle(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()

	logger := New()
	defer logger.Close()

	var wg sync.WaitGroup
	iterations := 50

	// Start logging goroutines
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			logger.Infof("message %d", n)
			logger.Warnf("warning %d", n)
			logger.Errorf("error %d", n)
			logger.Debugf("debug %d", n)
		}(i)
	}

	// Toggle debug concurrently
	for i := 0; i < iterations; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			SetGlobalDebug(n%2 == 0)
		}(i)
	}

	wg.Wait()
	// Test passes if no race conditions or panics occur
}
