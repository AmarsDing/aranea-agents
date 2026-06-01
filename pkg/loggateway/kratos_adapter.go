package loggateway

import (
	"github.com/go-kratos/kratos/v2/log"
	"go.uber.org/zap"
)

type KratosAdapter struct {
	sugar *zap.SugaredLogger
	base  []interface{}
}

func (a *KratosAdapter) Log(level log.Level, keyvals ...interface{}) error {
	if a == nil || a.sugar == nil {
		return nil
	}
	msg := extractMessage(keyvals)
	fields := kvToFields(keyvals)

	switch level {
	case log.LevelDebug:
		a.sugar.Debugw(msg, fields...)
	case log.LevelInfo:
		a.sugar.Infow(msg, fields...)
	case log.LevelWarn:
		a.sugar.Warnw(msg, fields...)
	case log.LevelError:
		a.sugar.Errorw(msg, fields...)
	default:
		a.sugar.Infow(msg, fields...)
	}
	return nil
}

func (a *KratosAdapter) WithFields(kv ...interface{}) *KratosAdapter {
	if a == nil {
		return &KratosAdapter{}
	}
	newBase := make([]interface{}, 0, len(a.base)+len(kv))
	newBase = append(newBase, a.base...)
	newBase = append(newBase, kv...)
	return &KratosAdapter{sugar: a.sugar, base: newBase}
}

func extractMessage(kv []interface{}) string {
	for i := 0; i+1 < len(kv); i += 2 {
		if kv[i] == "msg" {
			if s, ok := kv[i+1].(string); ok {
				return s
			}
		}
	}
	return ""
}

func kvToFields(kv []interface{}) []interface{} {
	out := make([]interface{}, 0, len(kv))
	for i := 0; i+1 < len(kv); i += 2 {
		key, ok := kv[i].(string)
		if !ok {
			continue
		}
		if key == "msg" || key == "ts" {
			continue
		}
		out = append(out, key, kv[i+1])
	}
	return out
}
