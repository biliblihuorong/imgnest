package repo

import (
	"context"
	"log/slog"
	"regexp"
	"time"

	"github.com/biliblihuorong/imgnest/internal/logx"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/utils"
)

// LogSlowQueries writes statements slower than threshold to log at warn
// level, with string literals blanked so stored values (emails, hashes,
// encrypted credentials) never reach the log. Zero leaves queries unlogged.
func LogSlowQueries(db *gorm.DB, log *slog.Logger, threshold time.Duration) {
	if threshold <= 0 || log == nil {
		return
	}
	db.Logger = newSlowQueryLogger(db, log, threshold)
}

func newSlowQueryLogger(db *gorm.DB, log *slog.Logger, threshold time.Duration) slowQueryLogger {
	literals := sqlString
	if db.Name() == "sqlite" {
		// GORM renders SQLite string values in double quotes; identifiers
		// use backticks there.
		literals = sqliteString
	}
	return slowQueryLogger{log: log, threshold: threshold, literals: literals}
}

type slowQueryLogger struct {
	log       *slog.Logger
	threshold time.Duration
	literals  *regexp.Regexp
}

var (
	sqlString    = regexp.MustCompile(`'(?:[^']|'')*'`)
	sqliteString = regexp.MustCompile(`'(?:[^']|'')*'|"(?:[^"]|"")*"`)
)

func (l slowQueryLogger) LogMode(logger.LogLevel) logger.Interface { return l }
func (l slowQueryLogger) Info(context.Context, string, ...any)     {}
func (l slowQueryLogger) Warn(context.Context, string, ...any)     {}
func (l slowQueryLogger) Error(context.Context, string, ...any)    {}

func (l slowQueryLogger) Trace(ctx context.Context, begin time.Time, fc func() (string, int64), _ error) {
	elapsed := time.Since(begin)
	if elapsed < l.threshold {
		return
	}
	statement, rows := fc()
	if len(statement) > 2000 {
		statement = statement[:2000] + "…"
	}
	l.log.WarnContext(ctx, "slow database query",
		"duration_ms", elapsed.Milliseconds(), "rows", rows,
		"sql", logx.Redact(l.literals.ReplaceAllString(statement, "'?'")),
		"source", utils.FileWithLineNum())
}
