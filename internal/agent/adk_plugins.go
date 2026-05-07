package agent

import (
	"os"
	"strings"

	"google.golang.org/adk/plugin"
	"google.golang.org/adk/plugin/loggingplugin"
	"google.golang.org/adk/plugin/retryandreflect"
)

// DefaultRunnerPlugins registers cross-cutting ADK plugins for in-process Runner instances.
func DefaultRunnerPlugins() []*plugin.Plugin {
	var out []*plugin.Plugin
	if strings.TrimSpace(os.Getenv("ARANEA_ADK_LOGGING_PLUGIN")) == "1" {
		if lp, err := loggingplugin.New("aranea"); err == nil {
			out = append(out, lp)
		}
	}
	rr, err := retryandreflect.New(
		retryandreflect.WithMaxRetries(5),
		retryandreflect.WithErrorIfRetryExceeded(false),
		retryandreflect.WithTrackingScope(retryandreflect.Invocation),
	)
	if err != nil {
		return out
	}
	return append(out, rr)
}
