package repl

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"

	"aranea-agents/internal/cli/client"
)

// renderState tracks the in-progress tool-call display.
type renderState struct {
	mu          sync.Mutex
	currentText strings.Builder
	toolBlocks  map[string]*toolBlock
	output      io.Writer
}

type toolBlock struct {
	name   string
	closed bool
}

func newRenderState(output io.Writer) *renderState {
	return &renderState{
		toolBlocks: make(map[string]*toolBlock),
		output:     output,
	}
}

// Handle processes a single downstream envelope and writes output.
func (rs *renderState) Handle(env client.Envelope) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	envType := env.Type

	// Downstream events arrive either as top-level type or inside envelope field.
	if envType == "" || envType == "server_to_client" {
		if inner, ok := env.Envelope["type"].(string); ok {
			envType = inner
		}
	}

	switch envType {
	case "text_delta":
		text := extractEnvelopeText(env)
		fmt.Fprint(rs.output, text)
		rs.currentText.WriteString(text)

	case "text_done":
		// End of text stream; print a newline if we have in-progress text.
		if rs.currentText.Len() > 0 {
			fmt.Fprintln(rs.output)
			rs.currentText.Reset()
		}

	case "tool_call":
		tc := extractToolCall(env)
		if tc == nil {
			return
		}
		id := tc["id"]
		name, _ := id.(string)
		tcName, _ := tc["name"].(string)
		if tcName != "" {
			name = tcName
		}
		status, _ := tc["status"].(string)
		callID, _ := tc["id"].(string)

		switch status {
		case "running", "":
			// Opening a tool call block
			if rs.currentText.Len() > 0 {
				fmt.Fprintln(rs.output)
				rs.currentText.Reset()
			}
			fmt.Fprintf(rs.output, "  ⚙ 调用工具 %s ...\n", name)
			rs.toolBlocks[callID] = &toolBlock{name: name}

		case "success":
			if blk, ok := rs.toolBlocks[callID]; ok && !blk.closed {
				fmt.Fprintf(rs.output, "  ✓ %s 完成\n", blk.name)
				blk.closed = true
			}

		case "error":
			if blk, ok := rs.toolBlocks[callID]; ok && !blk.closed {
				errMsg, _ := tc["error_code"].(string)
				fmt.Fprintf(rs.output, "  ✗ %s 失败: %s\n", blk.name, errMsg)
				blk.closed = true
			}
		}

	case "tool_result":
		tc := extractToolCall(env)
		if tc == nil {
			return
		}
		callID, _ := tc["id"].(string)
		if blk, ok := rs.toolBlocks[callID]; ok && !blk.closed {
			fmt.Fprintf(rs.output, "  ✓ %s 完成\n", blk.name)
			blk.closed = true
		}

	case "runner_completion", "run_status":
		if rs.currentText.Len() > 0 {
			fmt.Fprintln(rs.output)
			rs.currentText.Reset()
		}

	case "error":
		if rs.currentText.Len() > 0 {
			fmt.Fprintln(rs.output)
			rs.currentText.Reset()
		}
		errMsg := extractErrorMessage(env)
		fmt.Fprintf(rs.output, "\n[错误] %s\n", errMsg)

	case "transfer":
		payload := env.Envelope
		if payload == nil {
			payload = env.Payload
		}
		toAgent, _ := payload["transfer"].(map[string]any)
		if toAgent != nil {
			to, _ := toAgent["to_agent"].(string)
			if to != "" {
				fmt.Fprintf(rs.output, "\n[转接至 %s]\n", to)
			}
		}

	case "pong":
		// ignore heartbeat replies

	case "server_shutdown":
		fmt.Fprintln(rs.output, "\n[服务器关闭]")

	case "connected":
		// connection confirmation, ignore

	default:
		if envType != "" {
			fmt.Fprintf(rs.output, "\n[未识别事件: %s]\n", envType)
		}
	}
}

func extractEnvelopeText(env client.Envelope) string {
	// Try envelope.content.text first (downstream from server)
	if inner := env.Envelope; inner != nil {
		if content, ok := inner["content"].(map[string]any); ok {
			if t, ok := content["text"].(string); ok {
				return t
			}
		}
	}
	// Fall back to payload.text
	if env.Payload != nil {
		if t, ok := env.Payload["text"].(string); ok {
			return t
		}
	}
	return ""
}

func extractToolCall(env client.Envelope) map[string]any {
	inner := env.Envelope
	if inner == nil {
		inner = env.Payload
	}
	if tc, ok := inner["tool_call"].(map[string]any); ok {
		return tc
	}
	return nil
}

func extractErrorMessage(env client.Envelope) string {
	inner := env.Envelope
	if inner == nil {
		inner = env.Payload
	}
	if errObj, ok := inner["error"].(map[string]any); ok {
		if msg, ok := errObj["message"].(string); ok {
			return msg
		}
	}
	// Try raw JSON marshal for debugging
	b, _ := json.Marshal(inner)
	return string(b)
}
