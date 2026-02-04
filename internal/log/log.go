package log

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	logger      *zap.Logger
	enabled     bool
	initialized bool
	mu          sync.Mutex
	turnCount   int // Track conversation turns
)

// Init initializes the logger based on GEN_DEBUG env var
func Init() error {
	mu.Lock()
	defer mu.Unlock()

	if initialized {
		return nil
	}
	initialized = true

	if os.Getenv("GEN_DEBUG") != "1" {
		logger = zap.NewNop()
		return nil
	}

	enabled = true

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	// Create log directory
	logDir := filepath.Join(homeDir, ".gen")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return err
	}

	logPath := filepath.Join(logDir, "debug.log")

	// Use lumberjack for log rotation
	writeSyncer := zapcore.AddSync(&lumberjack.Logger{
		Filename:   logPath,
		MaxSize:    50,   // MB
		MaxBackups: 3,
		MaxAge:     7,    // Days
		Compress:   true,
	})

	// Console encoder for human-readable output
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "T",
		LevelKey:       "",  // Hide level, we use custom markers
		NameKey:        "",
		CallerKey:      "",  // Hide caller for cleaner output
		MessageKey:     "M",
		StacktraceKey:  "",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeTime:     zapcore.TimeEncoderOfLayout("15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
	}

	core := zapcore.NewCore(
		zapcore.NewConsoleEncoder(encoderConfig),
		writeSyncer,
		zapcore.DebugLevel,
	)

	logger = zap.New(core, zap.AddCaller())

	// Log initialization
	logger.Info("Debug logging started")

	return nil
}

// IsEnabled returns whether debug logging is enabled
func IsEnabled() bool {
	return enabled
}

// Logger returns the underlying zap logger
func Logger() *zap.Logger {
	if logger == nil {
		return zap.NewNop()
	}
	return logger
}

// Sync flushes any buffered log entries
func Sync() error {
	if logger != nil {
		return logger.Sync()
	}
	return nil
}

// NextTurn increments and returns the turn counter
func NextTurn() int {
	mu.Lock()
	defer mu.Unlock()
	turnCount++
	return turnCount
}

// CurrentTurn returns the current turn number
func CurrentTurn() int {
	mu.Lock()
	defer mu.Unlock()
	return turnCount
}

// escapeForLog escapes newlines and tabs for single-line log output
func escapeForLog(s string) string {
	s = strings.ReplaceAll(s, "\n", "\\n")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\t", "\\t")
	return s
}

// LogStreamDone logs stream completion stats
func LogStreamDone(provider string, duration time.Duration, chunks int) {
	if !enabled {
		return
	}
	logger.Info(fmt.Sprintf("[stream] %s done duration=%s chunks=%d", provider, duration.Round(time.Millisecond), chunks))
}

// LogTool logs tool execution with timing
func LogTool(name, id string, durationMs int64, success bool) {
	if !enabled {
		return
	}
	status := "ok"
	if !success {
		status = "error"
	}
	logger.Info(fmt.Sprintf("[tool] %s id=%s %dms %s", name, id, durationMs, status))
}
