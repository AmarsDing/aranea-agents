package plugintrpc

import (
	"context"
	"fmt"
	"os"
	"strings"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

var hookLogger = NewPluginSafeLogger("hook", nil)

func InitHookLogger(bus event.Bus) {
	hookLogger = NewPluginSafeLogger("hook", bus)
}

type PluginSafeLogger struct {
	pluginName string
	bus        event.Bus
}

func NewPluginSafeLogger(pluginName string, bus event.Bus) *PluginSafeLogger {
	return &PluginSafeLogger{pluginName: pluginName, bus: bus}
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
	var buf strings.Builder
	fmt.Fprintf(&buf, "[%s][%s] %s", level, l.pluginName, msg)
	for i := 0; i+1 < len(attrs); i += 2 {
		fmt.Fprintf(&buf, " %v=%v", attrs[i], attrs[i+1])
	}
	text := buf.String()

	fmt.Fprintln(os.Stderr, text)
	os.Stderr.Sync()

	if l.bus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeLog, l.pluginName, "")
		env.Channel = "monitor"
		env.Metadata = map[string]any{"level": level, "source": l.pluginName}
		env.Content = &event.EnvelopeContent{Text: text, IsPartial: false}
		bus := l.bus
		safego.Go(context.Background(), "plugin-log-"+l.pluginName, func() {
			bus.Publish(context.Background(), env)
		})
	}
}
