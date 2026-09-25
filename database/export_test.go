package database

import (
	"time"

	"github.com/jackc/pgx/v5/tracelog"
	"go.uber.org/zap"
)

// NewQueryTracerForTest exposes newQueryTracer to the external test package,
// whose frames (unlike package database's own) count as application code.
func NewQueryTracerForTest(log *zap.Logger, slow time.Duration) tracelog.Logger {
	return newQueryTracer(log, slow)
}
