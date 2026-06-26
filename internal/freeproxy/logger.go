// Package freeproxy — logger.go: structured in-memory log buffer with file persistence.
package freeproxy

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// LogLevel represents log severity.
type LogLevel string

const (
	LevelInfo  LogLevel = "info"
	LevelWarn  LogLevel = "warn"
	LevelError LogLevel = "error"
)

// LogEntry is a single log record.
type LogEntry struct {
	Time    string   `json:"time"`
	Level   LogLevel `json:"level"`
	Module  string   `json:"module"`
	Message string   `json:"message"`
}

// Logger is a ring-buffer logger with optional file persistence.
type Logger struct {
	mu       sync.RWMutex
	buf      []LogEntry
	cap      int
	pos      int
	total    int64
	filePath string
	file     *os.File
}

var defaultLogger *Logger
var logOnce sync.Once

// InitLogger initializes the global logger.
func InitLogger(logDir string, cap int) *Logger {
	logOnce.Do(func() {
		if cap <= 0 {
			cap = 500
		}
		l := &Logger{
			buf: make([]LogEntry, cap),
			cap: cap,
		}

		if logDir != "" {
			if err := os.MkdirAll(logDir, 0755); err == nil {
				path := filepath.Join(logDir, "proxypool.log")
				f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
				if err == nil {
					l.filePath = path
					l.file = f
				}
			}
		}

		defaultLogger = l
	})
	return defaultLogger
}

// GetLogger returns the global logger.
func GetLogger() *Logger {
	return defaultLogger
}

// Log adds a log entry.
func (l *Logger) Log(level LogLevel, module, format string, args ...any) {
	if l == nil {
		return
	}
	msg := fmt.Sprintf(format, args...)
	entry := LogEntry{
		Time:    time.Now().Format("2006-01-02 15:04:05"),
		Level:   level,
		Module:  module,
		Message: msg,
	}

	l.mu.Lock()
	l.buf[l.pos%l.cap] = entry
	l.pos++
	l.total++
	l.mu.Unlock()

	// Also write to file
	if l.file != nil {
		line := fmt.Sprintf("[%s] [%s] [%s] %s\n", entry.Time, entry.Level, entry.Module, entry.Message)
		l.file.WriteString(line)
	}

	// Also print to stderr for errors
	if level == LevelError {
		log.Printf("[%s] %s", module, msg)
	}
}

// Info logs at info level.
func (l *Logger) Info(module, format string, args ...any) {
	l.Log(LevelInfo, module, format, args...)
}

// Warn logs at warn level.
func (l *Logger) Warn(module, format string, args ...any) {
	l.Log(LevelWarn, module, format, args...)
}

// Error logs at error level.
func (l *Logger) Error(module, format string, args ...any) {
	l.Log(LevelError, module, format, args...)
}

// Query returns log entries matching the given filters.
func (l *Logger) Query(level LogLevel, limit int) []LogEntry {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()

	if limit <= 0 || limit > l.cap {
		limit = l.cap
	}

	var result []LogEntry
	// Walk from newest to oldest
	for i := l.pos - 1; i >= 0 && len(result) < limit; i-- {
		e := l.buf[i%l.cap]
		if e.Time == "" {
			break
		}
		if level != "" && e.Level != level {
			continue
		}
		result = append(result, e)
	}
	return result
}

// Stats returns total log count.
func (l *Logger) Stats() map[string]any {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return map[string]any{
		"total": l.total,
		"cap":   l.cap,
	}
}

// Close flushes and closes the log file.
func (l *Logger) Close() {
	if l != nil && l.file != nil {
		l.file.Close()
	}
}
