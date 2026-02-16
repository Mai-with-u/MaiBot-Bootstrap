package logging

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Options struct {
	FilePath       string
	MaxSizeMB      int
	RetentionDays  int
	MaxBackupFiles int
}

type Logger struct {
	zap *zap.SugaredLogger
}

func NewRoot(opts Options) (*Logger, error) {
	if strings.TrimSpace(opts.FilePath) == "" {
		return nil, fmt.Errorf("log file path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(opts.FilePath), 0o755); err != nil {
		return nil, err
	}
	if opts.MaxSizeMB <= 0 {
		opts.MaxSizeMB = 10
	}
	if opts.RetentionDays <= 0 {
		opts.RetentionDays = 7
	}
	if opts.MaxBackupFiles <= 0 {
		opts.MaxBackupFiles = 20
	}

	consoleEncoderCfg := zapcore.EncoderConfig{
		TimeKey:          "time",
		LevelKey:         "level",
		NameKey:          "module",
		MessageKey:       "msg",
		EncodeTime:       shortTimeEncoder,
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
		ConsoleSeparator: " | ",
	}
	if isTTY(os.Stdout) {
		consoleEncoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	} else {
		consoleEncoderCfg.EncodeLevel = zapcore.CapitalLevelEncoder
	}

	fileEncoderCfg := zapcore.EncoderConfig{
		TimeKey:          "time",
		LevelKey:         "level",
		NameKey:          "module",
		MessageKey:       "msg",
		EncodeTime:       longTimeEncoder,
		EncodeLevel:      zapcore.CapitalLevelEncoder,
		EncodeDuration:   zapcore.StringDurationEncoder,
		EncodeCaller:     zapcore.ShortCallerEncoder,
		ConsoleSeparator: " | ",
	}

	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(consoleEncoderCfg),
		zapcore.Lock(os.Stdout),
		zapcore.InfoLevel,
	)

	roller := &lumberjack.Logger{
		Filename:   opts.FilePath,
		MaxSize:    opts.MaxSizeMB,
		MaxBackups: opts.MaxBackupFiles,
		MaxAge:     opts.RetentionDays,
		Compress:   false,
	}
	fileCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(fileEncoderCfg),
		zapcore.AddSync(roller),
		zapcore.InfoLevel,
	)

	base := zap.New(zapcore.NewTee(consoleCore, fileCore), zap.AddCallerSkip(1))
	return &Logger{zap: base.Sugar()}, nil
}

func (l *Logger) Module(module string) *Logger {
	m := strings.TrimSpace(module)
	if m == "" {
		m = "app"
	}
	return &Logger{zap: l.zap.Named(m)}
}

func (l *Logger) Infof(format string, args ...any) {
	l.zap.Infof(format, args...)
}

func (l *Logger) Okf(format string, args ...any) {
	l.zap.Infof(format, args...)
}

func (l *Logger) Warnf(format string, args ...any) {
	l.zap.Warnf(format, args...)
}

func (l *Logger) Errorf(format string, args ...any) {
	l.zap.Errorf(format, args...)
}

func (l *Logger) Fatalf(format string, args ...any) {
	l.zap.Fatalf(format, args...)
}

func shortTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("01-02 15:04"))
}

func longTimeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05"))
}

func isTTY(f *os.File) bool {
	if f == nil {
		return false
	}
	st, err := f.Stat()
	if err != nil {
		return false
	}
	return (st.Mode() & os.ModeCharDevice) != 0
}
