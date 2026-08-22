package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools/patcherfs"
	"aranea-agents/pkg/loggateway"
)

const (
	siAnalystMaxToolRounds = 6
	siPatcherMaxToolRounds = 8
)

type siFinalParser[T any] func(text string) (T, error)

// siRunToolLoop drives a bounded LLM ↔ patcherfs loop. Each LLM call may
// return either a tool JSON ({"tool":...}) or the stage's final contract.
// Tool rounds do not consume the single format-correction retry.
func siRunToolLoop[T any](
	ctx context.Context,
	call func(context.Context, string) (string, error),
	ws *patcherfs.Workspace,
	user string,
	parse siFinalParser[T],
	maxToolRounds int,
	lg loggateway.Logger,
	step string,
) (T, error) {
	var zero T
	transcript := user
	toolRounds := 0
	formatRetries := 0
	var lastParse error
	for {
		text, err := call(ctx, transcript)
		if err != nil {
			if formatRetries > 0 && lastParse != nil {
				return zero, lastParse
			}
			return zero, err
		}
		if req, ok := patcherfs.ParseRequest(text); ok {
			if _, ferr := parse(text); ferr == nil {
				// A final contract that also happens to include "tool" wins.
				out, _ := parse(text)
				return out, nil
			}
			if toolRounds >= maxToolRounds {
				transcript = transcript + "\n\n[系统]工具轮次已用尽，请立即输出最终 JSON（不要再调用工具）。"
				toolRounds++
				if toolRounds > maxToolRounds+1 {
					return zero, fmt.Errorf("tool budget exhausted")
				}
				continue
			}
			result := "error: workspace not bound"
			if ws != nil {
				result = ws.Exec(req)
			}
			if lg != nil {
				lg.Info("si tool call",
					loggateway.StepID(step+".tool"),
					loggateway.Str("tool", req.Tool),
					loggateway.Str("path", req.Path))
			}
			transcript = fmt.Sprintf("%s\n\n[tool %s path=%s]\n%s\n请继续：需要时再调用一个工具，或输出最终 JSON。",
				transcript, req.Tool, req.Path, result)
			toolRounds++
			continue
		}
		out, perr := parse(text)
		if perr == nil {
			return out, nil
		}
		lastParse = perr
		if formatRetries >= 1 {
			return zero, perr
		}
		formatRetries++
		transcript = siFormatCorrection(transcript, text, perr)
	}
}

func siAnalystToolsHint(readRoot string) string {
	if strings.TrimSpace(readRoot) == "" {
		return ""
	}
	return fmt.Sprintf(`
可用只读工具（每轮只输出一个 JSON，不要夹杂其他文字）：
{"tool":"%s","path":"repo 相对路径"}
{"tool":"%s","path":"相对目录，空或 . 表示根"}
读完后输出 Diagnosis JSON（必须含 root_cause / fix_strategy，不要带 tool 字段）。
只读根：仓库工作树（%s）。禁止写文件。
`, patcherfs.ToolRead, patcherfs.ToolList, readRoot)
}

func siPatcherToolsHint(worktree string) string {
	if strings.TrimSpace(worktree) == "" {
		return ""
	}
	return fmt.Sprintf(`
可用 worktree 工具（每轮只输出一个 JSON）：
{"tool":"%s","path":"相对路径"}
{"tool":"%s","path":"相对目录"}
{"tool":"%s","path":"相对路径","content":"完整文件内容"}
{"tool":"%s","path":"可选，限制 diff 范围"}
写完后请调用 %s 核对，再输出最终 PatcherOutput JSON（必须含 diff 与 kind，不要带 tool 字段）。
worktree：%s
`, patcherfs.ToolRead, patcherfs.ToolList, patcherfs.ToolWrite, patcherfs.ToolDiff, patcherfs.ToolDiff, worktree)
}

func siMaybeFillDiffFromWorktree(ws *patcherfs.Workspace, text string, parsed *biz.PatcherOutput, perr error) (*biz.PatcherOutput, error) {
	if ws == nil {
		return parsed, perr
	}
	live, err := ws.Diff("")
	if err != nil || strings.TrimSpace(live) == "" {
		return parsed, perr
	}
	if parsed != nil && strings.TrimSpace(parsed.Diff) != "" {
		return parsed, nil
	}
	payload, err := json.Marshal(map[string]string{"diff": live, "kind": "code"})
	if err != nil {
		return parsed, perr
	}
	out, ferr := biz.ParsePatcherOutputJSON(string(payload))
	if ferr != nil {
		return parsed, perr
	}
	if parsed != nil {
		if parsed.Kind != "" {
			out.Kind = parsed.Kind
		}
		out.Summary = parsed.Summary
	}
	return out, nil
}
