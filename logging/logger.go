package logging

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// Logger 定义日志记录器接口
type Logger interface {
	Info(msg string, fields ...interface{})
	Warn(msg string, fields ...interface{})
	Error(msg string, fields ...interface{})
	Debug(msg string, fields ...interface{})
}

// Level 日志级别
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

// Format 日志格式
type Format int

const (
	FormatText Format = iota
	FormatJSON
)

// LogEntry 日志条目
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// DefaultLogger 默认日志记录器实现
// 从 controller/internal/logger/logger.go 提取
type DefaultLogger struct {
	level  Level
	format Format
	output io.Writer
	mu     sync.Mutex
}

// Config 日志配置
type Config struct {
	Level  string // "debug", "info", "warn", "error", "fatal"
	Format string // "text", "json"
	Output string // "stdout", "stderr", or file path
}

// NewLogger 创建新的日志记录器
func NewLogger(cfg *Config) (*DefaultLogger, error) {
	level := parseLevel(cfg.Level)
	format := parseFormat(cfg.Format)

	var output io.Writer
	switch cfg.Output {
	case "stdout", "":
		output = os.Stdout
	case "stderr":
		output = os.Stderr
	default:
		f, err := os.OpenFile(cfg.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return nil, fmt.Errorf("open log file: %w", err)
		}
		output = f
	}

	return &DefaultLogger{
		level:  level,
		format: format,
		output: output,
	}, nil
}

// parseLevel 解析日志级别字符串
func parseLevel(s string) Level {
	switch s {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	case "fatal":
		return LevelFatal
	default:
		return LevelInfo
	}
}

// parseFormat 解析日志格式字符串
func parseFormat(s string) Format {
	if s == "json" {
		return FormatJSON
	}
	return FormatText
}

// log 内部日志记录方法
func (l *DefaultLogger) log(level Level, msg string, fields ...interface{}) {
	if level < l.level {
		return
	}

	entry := LogEntry{
		Timestamp: time.Now().Format(time.RFC3339),
		Level:     levelString(level),
		Message:   msg,
		Fields:    make(map[string]interface{}),
	}

	// 解析 fields（key-value pairs）
	for i := 0; i < len(fields); i += 2 {
		if i+1 < len(fields) {
			key := fmt.Sprintf("%v", fields[i])
			entry.Fields[key] = fields[i+1]
		}
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	var output string
	if l.format == FormatJSON {
		data, _ := json.Marshal(entry)
		output = string(data)
	} else {
		output = fmt.Sprintf("[%s] %s: %s", entry.Timestamp, entry.Level, entry.Message)
		if len(entry.Fields) > 0 {
			output += fmt.Sprintf(" %v", entry.Fields)
		}
	}

	fmt.Fprintln(l.output, output)

	if level == LevelFatal {
		os.Exit(1)
	}
}

// levelString 将日志级别转换为字符串
func levelString(l Level) string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	default:
		return "UNKNOWN"
	}
}

// Debug 记录调试级别日志
func (l *DefaultLogger) Debug(msg string, fields ...interface{}) {
	l.log(LevelDebug, msg, fields...)
}

// Info 记录信息级别日志
func (l *DefaultLogger) Info(msg string, fields ...interface{}) {
	l.log(LevelInfo, msg, fields...)
}

// Warn 记录警告级别日志
func (l *DefaultLogger) Warn(msg string, fields ...interface{}) {
	l.log(LevelWarn, msg, fields...)
}

// Error 记录错误级别日志
func (l *DefaultLogger) Error(msg string, fields ...interface{}) {
	l.log(LevelError, msg, fields...)
}

// Fatal 记录致命错误日志并退出程序
func (l *DefaultLogger) Fatal(msg string, fields ...interface{}) {
	l.log(LevelFatal, msg, fields...)
}
