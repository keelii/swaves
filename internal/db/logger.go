package db

import (
	"context"
	"log"
	"strings"

	"github.com/simukti/sqldb-logger"
)

type SqlLogger struct{}

func (l *SqlLogger) Log(
	ctx context.Context,
	level sqldblogger.Level,
	msg string,
	data map[string]interface{},
) {
	sql, _ := data["query"].(string)
	args := data["args"]
	duration := data["duration"]

	sql = strings.ReplaceAll(sql, "\n", " ")
	sql = strings.Join(strings.Fields(sql), " ")

	log.Printf(
		"[SQL] %s | %v | %v",
		sql,
		args,
		duration,
	)
}
