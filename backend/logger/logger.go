package logger

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"agent_center/backend/paths"
)

type LogLevel string

const (
	LevelDebug   LogLevel = "DEBUG"
	LevelInfo    LogLevel = "INFO"
	LevelWarn    LogLevel = "WARN"
	LevelError   LogLevel = "ERROR"
	LevelSuccess LogLevel = "SUCCESS"
)

type LogEntry struct {
	Timestamp string   `json:"timestamp"`
	Level     LogLevel `json:"level"`
	Message   string   `json:"message"`
	Caller    string   `json:"caller,omitempty"`
}

type Logger struct {
	file   *os.File
	mu     sync.Mutex
	logDir string
}

var (
	globalLogger *Logger
	loggerOnce   sync.Once
)

func GetLogger() *Logger {
	loggerOnce.Do(func() {
		// Same data directory as the database and configs.
		logDir := paths.LogDir()
		_ = os.MkdirAll(logDir, 0755)

		filePath := filepath.Join(logDir, "agent_center.log")
		f, _ := os.OpenFile(filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)

		globalLogger = &Logger{
			file:   f,
			logDir: logDir,
		}
	})
	return globalLogger
}

func (l *Logger) Log(level LogLevel, msg string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     level,
		Message:   msg,
	}

	bytesData, err := json.Marshal(entry)
	if err == nil && l.file != nil {
		_, _ = l.file.Write(append(bytesData, '\n'))
	}
}

func Debug(msg string)   { GetLogger().Log(LevelDebug, msg) }
func Info(msg string)    { GetLogger().Log(LevelInfo, msg) }
func Warn(msg string)    { GetLogger().Log(LevelWarn, msg) }
func Error(msg string)   { GetLogger().Log(LevelError, msg) }
func Success(msg string) { GetLogger().Log(LevelSuccess, msg) }
