package logutil

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

var levelNames = map[Level]string{
	LevelDebug: "DEBUG",
	LevelInfo:  "INFO",
	LevelWarn:  "WARN",
	LevelError: "ERROR",
}

var levelColors = map[Level]string{
	LevelDebug: "\033[90m",
	LevelInfo:  "\033[32m",
	LevelWarn:  "\033[33m",
	LevelError: "\033[31m",
}

const reset = "\033[0m"
const dim = "\033[2m"

type Logger struct {
	mu    sync.Mutex
	level Level
	out   io.Writer
	color bool
}

func New(level Level) *Logger {
	l := &Logger{
		level: level,
		out:   os.Stderr,
	}
	if _, ok := os.LookupEnv("NO_COLOR"); !ok && os.Getenv("TERM") != "dumb" {
		l.color = true
	}
	return l
}

func NewDiscard() *Logger {
	return &Logger{level: LevelError + 1, out: io.Discard}
}

func (l *Logger) SetLevel(level Level) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.level = level
}

func (l *Logger) log(level Level, format string, args ...interface{}) {
	if level < l.level {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	msg := fmt.Sprintf(format, args...)
	ts := time.Now().Format("15:04:05")
	label := levelNames[level]

	if l.color {
		fmt.Fprintf(l.out, "%s%s%s %s%5s%s %s\n", dim, ts, reset, levelColors[level], label, reset, msg)
	} else {
		fmt.Fprintf(l.out, "%s %5s %s\n", ts, label, msg)
	}
}

func (l *Logger) Debug(format string, args ...interface{}) { l.log(LevelDebug, format, args...) }
func (l *Logger) Info(format string, args ...interface{})  { l.log(LevelInfo, format, args...) }
func (l *Logger) Warn(format string, args ...interface{})  { l.log(LevelWarn, format, args...) }
func (l *Logger) Error(format string, args ...interface{}) { l.log(LevelError, format, args...) }

func (l *Logger) Fatal(format string, args ...interface{}) {
	l.log(LevelError, format, args...)
	os.Exit(1)
}
