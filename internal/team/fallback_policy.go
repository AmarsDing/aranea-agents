package team

import (
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// FallbackDecision captures the outcome of the native-vs-graph runtime policy.
type FallbackDecision struct {
	UseNative      bool   // true → BuildTRPCTeam (emergency native path)
	MetricLabel    string // label for metrics: native_emergency | native_canary_holdout | native_fallback
	ErrorMessage   string // non-empty → no fallback available, caller must return error
}

// DecideNativeFallback determines whether a Team run should fall back to the
// native (BuildTRPCTeam) path when Graph compilation or build failed.
//
// Decision tree:
//  1. If ARANEA_TEAM_NATIVE=1 → always native (emergency).
//  2. If team is in canary holdout → native (canary_holdout).
//  3. Otherwise → error with a specific diagnostic message.
func DecideNativeFallback(
	def Definition,
	teamID string,
	graphAttempted bool,
	graphCompileErr string,
	graphBuildErr string,
	mode string,
	graphRootAvailable bool,
) FallbackDecision {
	canaryHoldout := teamNativeAllowedForCanaryHoldout(def, teamID)
	if envTeamNativeForced() || canaryHoldout {
		label := nativeRuntimeMetricReason(graphAttempted, canaryHoldout && !envTeamNativeForced())
		return FallbackDecision{
			UseNative:   true,
			MetricLabel: label,
		}
	}

	// No fallback available — produce a clear diagnostic error.
	msg := nativeFallbackDiagnosticMessage(def, teamID, graphCompileErr, graphBuildErr, mode, graphRootAvailable)
	return FallbackDecision{
		UseNative:    false,
		MetricLabel:  "",
		ErrorMessage: msg,
	}
}

// nativeFallbackDiagnosticMessage returns a human-readable diagnostic when
// neither Graph nor Native fallback is available.
func nativeFallbackDiagnosticMessage(
	def Definition,
	teamID string,
	graphCompileErr string,
	graphBuildErr string,
	mode string,
	graphRootAvailable bool,
) string {
	useGraph := TeamGraphRuntimeEnabled(def)
	switch {
	case !useGraph && strings.EqualFold(strings.TrimSpace(def.RuntimeEngine), "native"):
		return "team runtime_engine=native requires ARANEA_TEAM_NATIVE=1 or canary holdout (Graph is the default execution path)"
	case !useGraph && teamGraphCanaryPercent() < 100 && !teamInGraphCanaryBucket(teamID, teamGraphCanaryPercent()):
		return "team outside graph canary bucket; set runtime_engine=graph or ARANEA_TEAM_NATIVE=1"
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

// FallbackDecisionError converts a non-native FallbackDecision into a kratos error.
func (d FallbackDecision) Error() error {
	if d.ErrorMessage == "" {
		return nil
	}
	return kerrors.InternalServer("TEAM", d.ErrorMessage)
}
