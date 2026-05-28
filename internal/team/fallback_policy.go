package team

import (
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type FallbackDecision struct {
	UseNative    bool
	MetricLabel  string
	ErrorMessage string
}

func DecideNativeFallback(
	def Definition,
	teamID string,
	graphAttempted bool,
	graphCompileErr string,
	graphBuildErr string,
	mode string,
	graphRootAvailable bool,
) FallbackDecision {
	if envTeamNativeForced() {
		return FallbackDecision{
			UseNative:   true,
			MetricLabel: "native_emergency",
		}
	}

	msg := nativeFallbackDiagnosticMessage(graphCompileErr, graphBuildErr, mode, graphRootAvailable)
	return FallbackDecision{
		UseNative:    false,
		MetricLabel:  "",
		ErrorMessage: msg,
	}
}

func nativeFallbackDiagnosticMessage(graphCompileErr, graphBuildErr, mode string, graphRootAvailable bool) string {
	switch {
	case graphCompileErr != "":
		return "team graph compile failed: " + graphCompileErr
	case graphBuildErr != "":
		return "team graph build failed: " + graphBuildErr
	case !SupportsTeamGraphRuntimeMode(mode):
		return "team mode " + mode + " is not supported by graph runtime"
	case !graphRootAvailable:
		return "team graph runtime builder is not configured"
	default:
		return "team graph runtime unavailable"
	}
}

func (d FallbackDecision) Error() error {
	if d.ErrorMessage == "" {
		return nil
	}
	return kerrors.InternalServer("TEAM", d.ErrorMessage)
}
