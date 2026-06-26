package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

var (
	hookLoggerMu sync.RWMutex
	hookLogger   *PluginSafeLogger
)

// InitHookLogger initializes the global hook logger with the typed MonitorBus
// used for monitor-event publishing.
func InitHookLogger(monitorBus contract.MonitorBus, lg loggateway.Logger) {
	hookLoggerMu.Lock()
	hookLogger = NewPluginSafeLogger("hook", monitorBus, lg)
	hookLoggerMu.Unlock()
}

func getHookLogger() *PluginSafeLogger {
	hookLoggerMu.RLock()
	l := hookLogger
	hookLoggerMu.RUnlock()
	return l
}

type PluginSafeLogger struct {
	pluginName string
	monitorBus contract.MonitorBus
	lg         loggateway.Logger
}

func NewPluginSafeLogger(pluginName string, monitorBus contract.MonitorBus, lg loggateway.Logger) *PluginSafeLogger {
	return &PluginSafeLogger{pluginName: pluginName, monitorBus: monitorBus, lg: lg}
}

func (l *PluginSafeLogger) Info(msg string, attrs ...any) {
	l.write("INFO", msg, attrs...)
}

func (l *PluginSafeLogger) Warn(msg string, attrs ...any) {
	l.write("WARN", msg, attrs...)
}

func (l *PluginSafeLogger) Error(msg string, attrs ...any) {
	l.write("ERROR", msg, attrs...)
}

func (l *PluginSafeLogger) Debug(msg string, attrs ...any) {
	l.write("DEBUG", msg, attrs...)
}

func (l *PluginSafeLogger) write(level, msg string, attrs ...any) {
	stepID := "plugin." + l.pluginName
	allFields := make([]loggateway.Field, 0, 1+len(attrs)/2)
	allFields = append(allFields, loggateway.StepID(stepID))
	for i := 0; i+1 < len(attrs); i += 2 {
		key, _ := attrs[i].(string)
		if key != "" {
			allFields = append(allFields, loggateway.Any(key, attrs[i+1]))
		}
	}

	switch level {
	case "ERROR":
		l.lg.Error(msg, allFields...)
	case "WARN":
		l.lg.Warn(msg, allFields...)
	case "DEBUG":
		l.lg.Debug(msg, allFields...)
	default:
		l.lg.Info(msg, allFields...)
	}

	// Publish a MonitorEvent on the typed MonitorBus when wired. When the
	// MonitorBus is nil (e.g. plugin loggers created without a bus), the
	// loggateway.Logger call above is the only side effect.
	if l.monitorBus == nil {
		return
	}
	var buf strings.Builder
	buf.WriteString("[")
	buf.WriteString(level)
	buf.WriteString("][")
	buf.WriteString(l.pluginName)
	buf.WriteString("] ")
	buf.WriteString(msg)
	for i := 0; i+1 < len(attrs); i += 2 {
		k, _ := attrs[i].(string)
		buf.WriteString(" ")
		buf.WriteString(k)
		buf.WriteString("=")
		fmt.Fprintf(&buf, "%v", attrs[i+1])
	}
	ev := contract.NewMonitorEvent(contract.MonitorEventTypeLog, l.pluginName)
	ev.Level = level
	ev.Message = buf.String()
	ev.Metadata = map[string]any{"level": level, "source": l.pluginName}
	mb := l.monitorBus
	safego.Go(appctx.Ctx(), "plugin-log-"+l.pluginName, func() {
		mb.Publish(context.Background(), ev)
	})
}
