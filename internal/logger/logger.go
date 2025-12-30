package logger

import (
	"fmt"
	"os"
	"strings"

	"multicloud-exporter/internal/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

var Log *zap.SugaredLogger

func init() {
	// Default logger before initialization
	config := zap.NewDevelopmentConfig()
	logger, err := config.Build()
	if err != nil {
		// 如果初始化失败，输出到 stderr 并使用 no-op logger
		fmt.Fprintf(os.Stderr, "Failed to initialize default logger: %v\n", err)
		logger = zap.NewNop()
	}
	Log = logger.Sugar()
}

func Init(cfg *config.LogConfig) {
	if cfg == nil {
		return
	}

	var writeSyncer zapcore.WriteSyncer

	// Determine output
	var outputs []zapcore.WriteSyncer

	output := strings.ToLower(cfg.Output)
	switch output {
	case "", "stdout", "console":
		outputs = append(outputs, zapcore.AddSync(os.Stdout))
	case "file":
		if cfg.File != nil && cfg.File.Path != "" {
			outputs = append(outputs, getFileWriteSyncer(cfg.File))
		} else {
			// Fallback to stdout if file path missing
			outputs = append(outputs, zapcore.AddSync(os.Stdout))
		}
	case "both":
		outputs = append(outputs, zapcore.AddSync(os.Stdout))
		if cfg.File != nil && cfg.File.Path != "" {
			outputs = append(outputs, getFileWriteSyncer(cfg.File))
		}
	}

	writeSyncer = zapcore.NewMultiWriteSyncer(outputs...)

	// Encoder config
	encoderConfig := zap.NewProductionEncoderConfig()
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Format) == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// Level
	level := zap.NewAtomicLevel()
	if err := level.UnmarshalText([]byte(cfg.Level)); err != nil {
		level.SetLevel(zap.InfoLevel)
	}

	core := zapcore.NewCore(encoder, writeSyncer, level)

	// Add caller and stacktrace
	logger := zap.New(core, zap.AddCaller())
	zap.RedirectStdLog(logger)
	Log = logger.Sugar()
}

func getFileWriteSyncer(cfg *config.FileLogConfig) zapcore.WriteSyncer {
	lumberJackLogger := &lumberjack.Logger{
		Filename:   cfg.Path,
		MaxSize:    cfg.MaxSize,
		MaxBackups: cfg.MaxBackups,
		MaxAge:     cfg.MaxAge,
		Compress:   cfg.Compress,
	}
	return zapcore.AddSync(lumberJackLogger)
}

// Sync flushes any buffered log entries
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
