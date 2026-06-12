package team

import (
	"aranea-agents/pkg/apierror"
)

func graphRuntimeDiagnosticError(graphCompileErr, graphBuildErr, mode string, graphRootAvailable bool) error {
	msg := nativeFallbackDiagnosticMessage(graphCompileErr, graphBuildErr, mode, graphRootAvailable)
	return apierror.Internal(apierror.DomainTeam, msg)
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
