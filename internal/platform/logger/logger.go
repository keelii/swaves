package logger

import (
	"fmt"
	stdlog "log"
	"os"
	"time"
)

const logTimeFormat = "2006/01/02 15:04:05.000"

var std = stdlog.New(os.Stderr, "", 0)

func Fatal(format string, args ...any) {
	output("FATAL", format, args...)
	os.Exit(1)
}

func Error(format string, args ...any) {
	output("ERROR", format, args...)
}

func Warn(format string, args ...any) {
	output("WARN", format, args...)
}

func Info(format string, args ...any) {
	output("INFO", format, args...)
}

func output(level string, format string, args ...any) {
	message := format
	if len(args) > 0 {
		message = fmt.Sprintf(format, args...)
	}
	std.Printf("%s [%s] %s", time.Now().Format(logTimeFormat), level, message)
}
