package loggateway

import (
	"errors"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type Logger interface {
	Debug(msg string, fields ...Field)
	Info(msg string, fields ...Field)
	Warn(msg string, fields ...Field)
	Error(msg string, fields ...Field)
	With(fields ...Field) Logger
}

type Field = zap.Field

func StepID(v string) Field             { return zap.String("step_id", v) }
func SessionID(v string) Field          { return zap.String("session_id", v) }
func TraceID(v string) Field            { return zap.String("trace_id", v) }
func RunID(v string) Field              { return zap.String("run_id", v) }
func Domain(v string) Field             { return zap.String("domain", v) }
func AgentKey(v string) Field           { return zap.String("agent_key", v) }
func Phase(v string) Field              { return zap.String("phase", v) }
func Duration(ms int64) Field           { return zap.Int64("duration_ms", ms) }
func Source(v string) Field             { return zap.String("source", v) }
func Err(v error) Field                 { return errorChainField(v) }
func Str(k, v string) Field             { return zap.String(k, v) }
func Int(k string, v int) Field         { return zap.Int(k, v) }
func Int64(k string, v int64) Field     { return zap.Int64(k, v) }
func Float64(k string, v float64) Field { return zap.Float64(k, v) }
func Bool(k string, v bool) Field       { return zap.Bool(k, v) }
func Any(k string, v interface{}) Field { return zap.Any(k, v) }

func errorChainField(err error) Field {
	if err == nil {
		return zap.Skip()
	}
	chain := unwrapChain(err)
	if len(chain) == 1 {
		return zap.Error(err)
	}
	return zap.Array("error_chain", errorChainArray(chain))
}

type errorChainArray []error

func (a errorChainArray) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	for _, err := range a {
		enc.AppendString(err.Error())
	}
	return nil
}

func unwrapChain(err error) []error {
	var chain []error
	current := err
	for current != nil {
		chain = append(chain, current)
		current = errors.Unwrap(current)
	}
	return chain
}
