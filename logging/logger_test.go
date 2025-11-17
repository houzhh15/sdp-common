package logging

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "stdout logger",
			cfg: &Config{
				Level:  "info",
				Format: "text",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "json logger",
			cfg: &Config{
				Level:  "debug",
				Format: "json",
				Output: "stdout",
			},
			wantErr: false,
		},
		{
			name: "stderr logger",
			cfg: &Config{
				Level:  "error",
				Format: "text",
				Output: "stderr",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := NewLogger(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLogger() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && logger == nil {
				t.Error("NewLogger() returned nil logger")
			}
		})
	}
}

func TestDefaultLogger_TextFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := &DefaultLogger{
		level:  LevelDebug,
		format: FormatText,
		output: &buf,
	}

	logger.Info("test message", "key1", "value1", "key2", 123)

	output := buf.String()
	if !strings.Contains(output, "INFO") {
		t.Error("Expected INFO level in output")
	}
	if !strings.Contains(output, "test message") {
		t.Error("Expected message in output")
	}
	if !strings.Contains(output, "key1") || !strings.Contains(output, "value1") {
		t.Error("Expected fields in output")
	}
}

func TestDefaultLogger_JSONFormat(t *testing.T) {
	var buf bytes.Buffer
	logger := &DefaultLogger{
		level:  LevelDebug,
		format: FormatJSON,
		output: &buf,
	}

	logger.Info("test message", "key1", "value1", "key2", 123)

	var entry LogEntry
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("Failed to parse JSON: %v", err)
	}

	if entry.Level != "INFO" {
		t.Errorf("Expected level INFO, got %s", entry.Level)
	}
	if entry.Message != "test message" {
		t.Errorf("Expected message 'test message', got %s", entry.Message)
	}
	if entry.Fields["key1"] != "value1" {
		t.Error("Expected field key1=value1")
	}
	if entry.Fields["key2"].(float64) != 123 {
		t.Error("Expected field key2=123")
	}
}

func TestDefaultLogger_LevelFiltering(t *testing.T) {
	tests := []struct {
		name      string
		logLevel  Level
		logFunc   func(*DefaultLogger, *bytes.Buffer)
		shouldLog bool
	}{
		{
			name:     "debug level - debug message",
			logLevel: LevelDebug,
			logFunc: func(l *DefaultLogger, buf *bytes.Buffer) {
				l.Debug("debug msg")
			},
			shouldLog: true,
		},
		{
			name:     "info level - debug message",
			logLevel: LevelInfo,
			logFunc: func(l *DefaultLogger, buf *bytes.Buffer) {
				l.Debug("debug msg")
			},
			shouldLog: false,
		},
		{
			name:     "info level - info message",
			logLevel: LevelInfo,
			logFunc: func(l *DefaultLogger, buf *bytes.Buffer) {
				l.Info("info msg")
			},
			shouldLog: true,
		},
		{
			name:     "warn level - info message",
			logLevel: LevelWarn,
			logFunc: func(l *DefaultLogger, buf *bytes.Buffer) {
				l.Info("info msg")
			},
			shouldLog: false,
		},
		{
			name:     "warn level - error message",
			logLevel: LevelWarn,
			logFunc: func(l *DefaultLogger, buf *bytes.Buffer) {
				l.Error("error msg")
			},
			shouldLog: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := &DefaultLogger{
				level:  tt.logLevel,
				format: FormatText,
				output: &buf,
			}

			tt.logFunc(logger, &buf)

			hasOutput := buf.Len() > 0
			if hasOutput != tt.shouldLog {
				t.Errorf("Expected shouldLog=%v, but got output=%v", tt.shouldLog, hasOutput)
			}
		})
	}
}

func TestDefaultLogger_AllLevels(t *testing.T) {
	var buf bytes.Buffer
	logger := &DefaultLogger{
		level:  LevelDebug,
		format: FormatText,
		output: &buf,
	}

	logger.Debug("debug message")
	if !strings.Contains(buf.String(), "DEBUG") {
		t.Error("Expected DEBUG in output")
	}

	buf.Reset()
	logger.Info("info message")
	if !strings.Contains(buf.String(), "INFO") {
		t.Error("Expected INFO in output")
	}

	buf.Reset()
	logger.Warn("warn message")
	if !strings.Contains(buf.String(), "WARN") {
		t.Error("Expected WARN in output")
	}

	buf.Reset()
	logger.Error("error message")
	if !strings.Contains(buf.String(), "ERROR") {
		t.Error("Expected ERROR in output")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"fatal", LevelFatal},
		{"invalid", LevelInfo}, // default to Info
		{"", LevelInfo},        // default to Info
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input)
			if got != tt.want {
				t.Errorf("parseLevel(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFormat(t *testing.T) {
	tests := []struct {
		input string
		want  Format
	}{
		{"json", FormatJSON},
		{"text", FormatText},
		{"invalid", FormatText}, // default to Text
		{"", FormatText},        // default to Text
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseFormat(tt.input)
			if got != tt.want {
				t.Errorf("parseFormat(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestDefaultLogger_ConcurrentWrites(t *testing.T) {
	var buf bytes.Buffer
	logger := &DefaultLogger{
		level:  LevelInfo,
		format: FormatText,
		output: &buf,
	}

	// 并发写入测试
	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			logger.Info("concurrent message", "n", n)
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// 验证没有数据竞争和崩溃
	if buf.Len() == 0 {
		t.Error("Expected some output from concurrent writes")
	}
}
