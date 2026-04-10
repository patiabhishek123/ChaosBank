package service

import (
	"encoding/json"
	"io"
	"os"
	"sync"
	"time"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

type Logger struct {
	out   io.Writer
	level Level
	mu    sync.Mutex
}

func NewLogger(level string, out io.Writer) *Logger {
	return &Logger{
		out:   out,
		level: ParseLevel(level),
	}
}

func ParseLevel(value string) Level {
	switch value {
	case "debug", "DEBUG":
		return Debug
	case "warn", "WARN", "warning", "WARNING":
		return Warn
	case "error", "ERROR":
		return Error
	default:
		return Info
	}
}

func (l Level) String() string {
	switch l {
	case Debug:
		return "debug"
	case Warn:
		return "warn"
	case Error:
		return "error"
	default:
		return "info"
	}
}

func (l *Logger) Log(level Level, message string, fields map[string]interface{}) {
	if level < l.level {
		return
	}

	entry := map[string]interface{}{
		"level": level.String(),
		"ts":    time.Now().UTC().Format(time.RFC3339Nano),
		"msg":   message,
	}

	for k, v := range fields {
		entry[k] = v
	}

	payload, err := json.Marshal(entry)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.out.Write(payload)
	l.out.Write([]byte("\n"))
}

func (l *Logger) Debug(message string, fields map[string]interface{}) {
	l.Log(Debug, message, fields)
}

func (l *Logger) Info(message string, fields map[string]interface{}) {
	l.Log(Info, message, fields)
}

func (l *Logger) Warn(message string, fields map[string]interface{}) {
	l.Log(Warn, message, fields)
}

func (l *Logger) Error(message string, fields map[string]interface{}) {
	l.Log(Error, message, fields)
}

func defaultOutput() io.Writer {
	return os.Stdout
}
