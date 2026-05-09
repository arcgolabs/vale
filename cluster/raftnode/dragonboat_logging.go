package raftnode

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	dragonlogger "github.com/lni/dragonboat/v3/logger"
)

var dragonboatLogger atomic.Pointer[slog.Logger]
var dragonboatLoggerFactoryConflict atomic.Bool
var dragonboatLoggerFactoryConflictReported atomic.Bool

func init() {
	dragonboatLogger.Store(slog.Default())
	defer func() {
		if recovered := recover(); recovered != nil {
			dragonboatLoggerFactoryConflict.Store(true)
		}
	}()
	dragonlogger.SetLoggerFactory(func(pkgName string) dragonlogger.ILogger {
		return newDragonboatSlogLogger(pkgName)
	})
}

func configureDragonboatLogger(logger *slog.Logger) {
	if logger == nil {
		return
	}
	dragonboatLogger.Store(logger)
	if dragonboatLoggerFactoryConflict.Load() && !dragonboatLoggerFactoryConflictReported.Swap(true) {
		logger.Warn("dragonboat logger factory is already configured; slog bridge was not installed")
	}
}

type dragonboatSlogLogger struct {
	pkgName string
	level   atomic.Int64
}

func newDragonboatSlogLogger(pkgName string) *dragonboatSlogLogger {
	logger := &dragonboatSlogLogger{pkgName: pkgName}
	logger.SetLevel(dragonlogger.INFO)
	return logger
}

func (l *dragonboatSlogLogger) SetLevel(level dragonlogger.LogLevel) {
	l.level.Store(int64(level))
}

func (l *dragonboatSlogLogger) Debugf(format string, args ...any) {
	l.logf(slog.LevelDebug, dragonlogger.DEBUG, format, args...)
}

func (l *dragonboatSlogLogger) Infof(format string, args ...any) {
	l.logf(slog.LevelInfo, dragonlogger.INFO, format, args...)
}

func (l *dragonboatSlogLogger) Warningf(format string, args ...any) {
	l.logf(slog.LevelWarn, dragonlogger.WARNING, format, args...)
}

func (l *dragonboatSlogLogger) Errorf(format string, args ...any) {
	l.logf(slog.LevelError, dragonlogger.ERROR, format, args...)
}

func (l *dragonboatSlogLogger) Panicf(format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	l.log(slog.LevelError, dragonlogger.CRITICAL, message)
	panic(message)
}

func (l *dragonboatSlogLogger) logf(level slog.Level, dragonLevel dragonlogger.LogLevel, format string, args ...any) {
	l.log(level, dragonLevel, fmt.Sprintf(format, args...))
}

func (l *dragonboatSlogLogger) log(level slog.Level, dragonLevel dragonlogger.LogLevel, message string) {
	if l == nil || !l.enabled(dragonLevel) {
		return
	}
	logger := dragonboatLogger.Load()
	if logger == nil {
		return
	}
	logger.LogAttrs(context.Background(), level, message, slog.String("component", "dragonboat"), slog.String("package", l.pkgName))
}

func (l *dragonboatSlogLogger) enabled(level dragonlogger.LogLevel) bool {
	return l != nil && int64(level) <= l.level.Load()
}
