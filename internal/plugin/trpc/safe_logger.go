package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

var (
	hookLoggerMu sync.RWMutex
	hookLogger   *PluginSafeLogger
)

// InitHookLogger initializes the global hook logger with both the legacy
// envelope bus (retained for backward compatibility) and the new typed
// MonitorBus (preferred for monitor-event publishing).
func InitHookLogger(bus event.Bus, monitorBus contract.MonitorBus, lg loggateway.Logger) {
	hookLoggerMu.Lock()
	hookLogger = NewPluginSafeLogger("hook", bus, lg)
	hookLogger.SetMonitorBus(monitorBus)
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
	bus        event.Bus           // legacy envelope bus (fallback when monitorBus is nil)
	monitorBus contract.MonitorBus // typed monitor bus (preferred)
	lg         loggateway.Logger
}

func NewPluginSafeLogger(pluginName string, bus event.Bus, lg loggateway.Logger) *PluginSafeLogger {
	return &PluginSafeLogger{pluginName: pluginName, bus: bus, lg: lg}
}

// SetMonitorBus attaches a typed MonitorBus. When set, the logger publishes
// MonitorEvents on the typed bus instead of legacy Envelopes on event.Bus.
func (l *PluginSafeLogger) SetMonitorBus(mb contract.MonitorBus) {
	if l == nil {
		return
	}
	l.monitorBus = mb
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

	// Dual-bus migration: prefer the typed MonitorBus. Fall back to the legacy
	// envelope bus when MonitorBus is not wired (e.g. plugin loggers created
	// via newBasePlugin before SetMonitorBus is called).
	if l.monitorBus != nil {
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
		return
	}
	if l.bus != nil {
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
		text := buf.String()
		env := event.NewEnvelope(event.EnvelopeTypeLog, l.pluginName, "")
		env.Channel = "monitor"
		env.Metadata = map[string]any{"level": level, "source": l.pluginName}
		env.Content = &event.EnvelopeContent{Text: text, IsPartial: false}
		bus := l.bus
		safego.Go(appctx.Ctx(), "plugin-log-"+l.pluginName, func() {
			bus.Publish(context.Background(), env)
		})
	}
}
