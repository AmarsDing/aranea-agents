package tools

import (
	"bytes"
	"strings"
	"sync/atomic"

	"aranea-agents/internal/tools/argnorm"
	"aranea-agents/internal/tools/filenorm"
	"aranea-agents/internal/tools/hostexecnorm"
	"aranea-agents/pkg/loggateway"
)

var aliasRewriteTotal atomic.Int64

// AliasRewriteTotal is the process count of NormalizeInvocation calls that
// actually rewrote arguments. Use it to measure alias traffic before retiring
// a compatibility mapping.
func AliasRewriteTotal() int64 {
	return aliasRewriteTotal.Load()
}

// NormalizeInvocation maps LLM/catalog aliases onto the live tool schema
// before locks, cache keys, and inner Call see the payload.
//
// This is the single product-layer entry: hostexec argv/cmd, file
// path/content/search, and web_fetch/search query aliases. Unknown tools
// pass through. Applying it twice is idempotent.
func NormalizeInvocation(toolName string, jsonArgs []byte) []byte {
	return normalizeInvocation(nil, toolName, jsonArgs)
}

// NormalizeInvocationWithLog is NormalizeInvocation plus a Debug line when
// aliases were rewritten (no argument payload — those can be large or secret).
func NormalizeInvocationWithLog(lg loggateway.Logger, toolName string, jsonArgs []byte) []byte {
	return normalizeInvocation(lg, toolName, jsonArgs)
}

func normalizeInvocation(lg loggateway.Logger, toolName string, jsonArgs []byte) []byte {
	if len(jsonArgs) == 0 {
		return jsonArgs
	}
	orig := jsonArgs
	name := canonicalRuntimeName(strings.TrimSpace(toolName))
	if registryNameFor(name) == "hostexec" || name == "exec_command" || name == "write_stdin" || name == "kill_session" {
		jsonArgs = hostexecnorm.NormalizeExecArgs(jsonArgs)
	}
	jsonArgs = filenorm.NormalizeFileArgs(name, jsonArgs)
	jsonArgs = argnorm.NormalizeArgs(name, jsonArgs)
	if !bytes.Equal(orig, jsonArgs) {
		aliasRewriteTotal.Add(1)
		if lg != nil {
			lg.Debug("工具参数已归一化别名",
				loggateway.StepID("tool.args.normalized"),
				loggateway.Str("tool", name))
		}
	}
	return jsonArgs
}
