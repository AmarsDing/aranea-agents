package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/callbacks"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// failedToolResultDedupHookPriority sits with prune (7): after injection
// (≤6), before assembly (8). Collapsing duplicate failures is always on
// even when blob prune is disabled — S05 GNS3 400 rows are small and
// skipped by the size/Error: exemptions.
const failedToolResultDedupHookPriority = 7

const failedToolResultDedupStubPrefix = "[duplicate failed tool_result"

// newFailedToolResultDedupBeforeHook collapses consecutive identical
// failed RoleTool payloads (keep first; stub 2nd+). Pair structure is
// preserved: tool_call messages are not deleted or reordered.
func newFailedToolResultDedupBeforeHook(lg loggateway.Logger) callbacks.Callback {
	return callbacks.NewBeforeModelHook(failedToolResultDedupHookPriority, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		collapsed := collapseConsecutiveFailedToolResults(args.Request.Messages)
		if collapsed > 0 {
			lg.Info("collapsed consecutive failed tool results",
				loggateway.StepID("agent.tool_result.fail_dedup"),
				loggateway.Int("collapsed", collapsed))
		}
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}

func collapseConsecutiveFailedToolResults(msgs []trpcmodel.Message) int {
	prevFP := ""
	collapsed := 0
	for i := range msgs {
		msg := &msgs[i]
		if msg.Role == trpcmodel.RoleUser && !isDynamicCueMessage(*msg) {
			prevFP = ""
			continue
		}
		if msg.Role != trpcmodel.RoleTool {
			continue
		}
		content := extractTextContent(msg)
		if strings.HasPrefix(strings.TrimSpace(content), failedToolResultDedupStubPrefix) {
			continue
		}
		if !isFailedToolResultContent(content) {
			prevFP = ""
			continue
		}
		fp := msg.ToolName + "\x00" + failedToolResultFingerprint(content)
		if prevFP != "" && fp == prevFP {
			msg.Content = fmt.Sprintf("%s｜same as previous｜tool=%s]", failedToolResultDedupStubPrefix, msg.ToolName)
			msg.ContentParts = nil
			collapsed++
			continue
		}
		prevFP = fp
	}
	return collapsed
}

func isFailedToolResultContent(content string) bool {
	s := strings.TrimSpace(content)
	if s == "" {
		return false
	}
	if strings.HasPrefix(s, "Error:") {
		return true
	}
	low := strings.ToLower(s)
	if strings.Contains(low, `"ok": false`) || strings.Contains(low, `"ok":false`) {
		return true
	}
	if strings.Contains(low, "empty command") || strings.Contains(s, "cmd 不能为空") {
		return true
	}
	return toolResultHTTPStatus(s) >= 400
}

func failedToolResultFingerprint(content string) string {
	s := strings.TrimSpace(content)
	if class := classifyFailedToolResult(s); class != "" {
		return class
	}
	if len(s) > 240 {
		s = s[:240]
	}
	return s
}

func classifyFailedToolResult(s string) string {
	low := strings.ToLower(s)
	if strings.Contains(low, "empty command") || strings.Contains(s, "cmd 不能为空") {
		return "empty_command"
	}
	if st := toolResultHTTPStatus(s); st >= 400 {
		return fmt.Sprintf("http_%d", st)
	}
	if strings.HasPrefix(strings.TrimSpace(s), "Error:") {
		line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
		if len(line) > 80 {
			line = line[:80]
		}
		return strings.ToLower(line)
	}
	return ""
}

func toolResultHTTPStatus(s string) int {
	var probe struct {
		HTTPStatus json.RawMessage `json:"httpStatus"`
		Result     *struct {
			HTTPStatus json.RawMessage `json:"httpStatus"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(s), &probe); err != nil {
		return 0
	}
	if n := atoiLoose(probe.HTTPStatus); n > 0 {
		return n
	}
	if probe.Result != nil {
		return atoiLoose(probe.Result.HTTPStatus)
	}
	return 0
}

func atoiLoose(raw json.RawMessage) int {
	if len(raw) == 0 {
		return 0
	}
	var n int
	if err := json.Unmarshal(raw, &n); err == nil {
		return n
	}
	var f float64
	if err := json.Unmarshal(raw, &f); err == nil {
		return int(f)
	}
	return 0
}
