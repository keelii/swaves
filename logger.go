package main

import (
	"io"
	"log"
	"os"
	"time"
)

const logTimeFormat = "2006/01/02 15:04:05.000"

type logWriter struct {
	w io.Writer
}

func (l *logWriter) Write(p []byte) (n int, err error) {
	ts := time.Now().Format(logTimeFormat)
	line := ts + " " + string(p)
	_, err = l.w.Write([]byte(line))
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func init() {
	log.SetFlags(0)
	log.SetOutput(&logWriter{w: os.Stderr})
}
