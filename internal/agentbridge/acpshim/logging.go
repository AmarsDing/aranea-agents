package acpshim

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"aranea-agents/pkg/loggateway"
)

// logging.go — shim 进程的日志出口（M4 设计 §D-1：stdout 纯净红线）。
//
//	stdout 只写 NDJSON 协议帧；一切日志经 loggateway.Logger 构造注入
//	走 stderr sink（红线 #16：禁止 log/slog）。

// NewStderrLogger 构造写 stderr 的 JSON 日志器（zap 实现 loggateway.Logger）。
// level 空/未知按 info；进程退出前调用方无需显式 Sync（stderr 即时落盘）。
func NewStderrLogger(component, level string) loggateway.Logger {
	lv := zapcore.InfoLevel
	if level != "" {
		if err := lv.UnmarshalText([]byte(strings.ToLower(level))); err != nil {
			lv = zapcore.InfoLevel
		}
	}
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "ts"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encCfg), zapcore.Lock(os.Stderr), lv)
	z := zap.New(core).With(zap.String("component", component))
	return &stderrLogger{z: z}
}

// stderrLogger 用 zap 实现 loggateway.Logger（Field = zap.Field 直通）。
type stderrLogger struct {
	z *zap.Logger
}

func (l *stderrLogger) Debug(msg string, fields ...loggateway.Field) {
	l.z.Debug(msg, fields...)
}

func (l *stderrLogger) Info(msg string, fields ...loggateway.Field) {
	l.z.Info(msg, fields...)
}

func (l *stderrLogger) Warn(msg string, fields ...loggateway.Field) {
	l.z.Warn(msg, fields...)
}

func (l *stderrLogger) Error(msg string, fields ...loggateway.Field) {
	l.z.Error(msg, fields...)
}

func (l *stderrLogger) With(fields ...loggateway.Field) loggateway.Logger {
	return &stderrLogger{z: l.z.With(fields...)}
}
