package db

import (
	"context"
	"strings"
	"swaves/internal/platform/logger"

	"github.com/simukti/sqldb-logger"
)

type SqlLogger struct {
	Enabled bool
}

func (l *SqlLogger) Log(
	ctx context.Context,
	level sqldblogger.Level,
	msg string,
	data map[string]interface{},
) {
	if !l.Enabled {
		return
	}
	sql, _ := data["query"].(string)
	args := data["args"]
	duration := data["duration"]

	sql = strings.ReplaceAll(sql, "\n", " ")
	sql = strings.Join(strings.Fields(sql), " ")

	logger.Info(
		"[SQL] %s | %v | %v",
		sql,
		args,
		duration,
	)
}
