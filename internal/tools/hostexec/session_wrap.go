package hostexec

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	maxLogBytes      = 1 << 20
	maxStreamRunes   = 4000
	notifyPollEvery  = 200 * time.Millisecond
	defaultNotifyWait = 30 * time.Second
)

type sessionToolSet struct {
	inner   trpctool.ToolSet
	baseDir string
	lg      loggateway.Logger
}

// WrapSessionEnhance adds output files, regex notify, and StreamableCall polling
// around hostexec exec_command. Confirmation and Exclusive locks stay on the
// decorator outside this wrap.
func WrapSessionEnhance(inner trpctool.ToolSet, baseDir string, lg loggateway.Logger) trpctool.ToolSet {
	if inner == nil {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &sessionToolSet{inner: inner, baseDir: baseDir, lg: lg}
}

func (s *sessionToolSet) Name() string {
	if s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *sessionToolSet) Close() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *sessionToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	var exec, stdin trpctool.CallableTool
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		ct, ok := t.(trpctool.CallableTool)
		if !ok {
			out[i] = t
			continue
		}
		name := toolName(ct)
		switch name {
		case "exec_command":
			exec = ct
		case "write_stdin":
			stdin = ct
		}
		out[i] = t
	}
	if exec == nil {
		return out
	}
	wrapped := &execEnhance{inner: exec, stdin: stdin, set: s}
	for i, t := range out {
		ct, ok := t.(trpctool.CallableTool)
		if ok && toolName(ct) == "exec_command" {
			out[i] = wrapped
		}
	}
	return out
}

type execEnhance struct {
	inner trpctool.CallableTool
	stdin trpctool.CallableTool
	set   *sessionToolSet
}

func (e *execEnhance) Declaration() *trpctool.Declaration {
	if e.inner == nil {
		return nil
	}
	d := e.inner.Declaration()
	if d == nil || d.InputSchema == nil {
		return d
	}
	cp := *d
	schema := *d.InputSchema
	props := map[string]*trpctool.Schema{}
	for k, v := range d.InputSchema.Properties {
		props[k] = v
	}
	if _, ok := props["notify_pattern"]; !ok {
		props["notify_pattern"] = &trpctool.Schema{
			Type:        "string",
			Description: "Regex to wait for in command output before returning. Alias: notify_on_output.",
		}
	}
	if _, ok := props["block_until_ms"]; !ok {
		props["block_until_ms"] = &trpctool.Schema{
			Type:        "integer",
			Description: "Alias for yield_time_ms: wait this many milliseconds before returning a running session.",
		}
	}
	schema.Properties = props
	cp.InputSchema = &schema
	if cp.Description != "" && !strings.Contains(cp.Description, "output_file") {
		cp.Description += " Long-running commands return session_id plus output_file under .aranea/shell/. Use notify_pattern to wait until output matches a regex. write_stdin polls the same session."
	}
	return &cp
}

func (e *execEnhance) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	pattern, wait := extractNotify(jsonArgs)
	out, err := e.inner.Call(ctx, jsonArgs)
	if err != nil {
		return e.persist(out, ""), err
	}
	m := asMap(out)
	sid, _ := m["session_id"].(string)
	output, _ := m["output"].(string)
	status, _ := m["status"].(string)
	if pattern != "" && e.stdin != nil && sid != "" {
		matched, acc, waitErr := e.waitPattern(ctx, sid, output, pattern, wait)
		output = acc
		m["output"] = output
		m["notified"] = matched
		if status == "" {
			status = "running"
		}
		m["status"] = status
		if waitErr != nil && ctx.Err() != nil {
			return e.persist(m, sid), waitErr
		}
	}
	return e.persist(m, sid), nil
}

func (e *execEnhance) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	stream := trpctool.NewStream(16)
	safego.Go(ctx, "hostexec.stream", func() {
		defer stream.Writer.Close()
		out, err := e.Call(ctx, jsonArgs)
		if err != nil {
			_ = stream.Writer.Send(trpctool.StreamChunk{Content: err.Error()}, err)
			return
		}
		m := asMap(out)
		sid, _ := m["session_id"].(string)
		output, _ := m["output"].(string)
		if output != "" {
			if stream.Writer.Send(trpctool.StreamChunk{Content: clipRunes(output, maxStreamRunes)}, nil) {
				return
			}
		}
		if sid == "" || e.stdin == nil {
			_ = stream.Writer.Send(trpctool.StreamChunk{Content: m}, nil)
			return
		}
		seen := output
		ticker := time.NewTicker(notifyPollEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				poll, perr := e.stdin.Call(ctx, mustJSON(map[string]any{
					"session_id":    sid,
					"chars":         "",
					"yield_time_ms": 0,
				}))
				if perr != nil {
					_ = stream.Writer.Send(trpctool.StreamChunk{Content: perr.Error()}, perr)
					return
				}
				pm := asMap(poll)
				chunk, _ := pm["output"].(string)
				st, _ := pm["status"].(string)
				if chunk != "" && chunk != seen {
					delta := chunk
					if strings.HasPrefix(chunk, seen) {
						delta = chunk[len(seen):]
					}
					seen = chunk
					if delta != "" {
						if stream.Writer.Send(trpctool.StreamChunk{Content: clipRunes(delta, maxStreamRunes)}, nil) {
							return
						}
					}
				}
				if st == "exited" {
					e.persist(mergeSession(m, pm, sid), sid)
					return
				}
			}
		}
	})
	return stream.Reader, nil
}

func (e *execEnhance) waitPattern(ctx context.Context, sessionID, initial, pattern string, wait time.Duration) (bool, string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, initial, nil
	}
	acc := initial
	if re.MatchString(acc) {
		return true, acc, nil
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if ctx.Err() != nil {
			return false, acc, ctx.Err()
		}
		timer := time.NewTimer(notifyPollEvery)
		select {
		case <-ctx.Done():
			timer.Stop()
			return false, acc, ctx.Err()
		case <-timer.C:
		}
		poll, perr := e.stdin.Call(ctx, mustJSON(map[string]any{
			"session_id":    sessionID,
			"chars":         "",
			"yield_time_ms": 0,
		}))
		if perr != nil {
			return false, acc, nil
		}
		pm := asMap(poll)
		chunk, _ := pm["output"].(string)
		if chunk != "" {
			if strings.HasPrefix(chunk, acc) {
				acc = chunk
			} else {
				acc += chunk
			}
		}
		if re.MatchString(acc) {
			return true, acc, nil
		}
		if st, _ := pm["status"].(string); st == "exited" {
			return re.MatchString(acc), acc, nil
		}
	}
	return false, acc, nil
}

func (e *execEnhance) persist(result any, sessionID string) any {
	m := asMap(result)
	if m == nil {
		return result
	}
	output, _ := m["output"].(string)
	if sessionID == "" {
		if s, ok := m["session_id"].(string); ok {
			sessionID = s
		}
	}
	if sessionID == "" && output == "" {
		return m
	}
	path, err := writeShellLog(e.set.baseDir, sessionID, output)
	if err != nil {
		e.set.lg.Debug("shell output file skipped",
			loggateway.StepID("tool.shell.output_file"),
			loggateway.Err(err))
		return m
	}
	if path != "" {
		m["output_file"] = path
	}
	return m
}

func writeShellLog(baseDir, sessionID, output string) (string, error) {
	if strings.TrimSpace(baseDir) == "" {
		return "", nil
	}
	dir := filepath.Join(baseDir, ".aranea", "shell")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	name := sessionID
	if name == "" {
		name = "foreground"
	}
	name = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return '_'
		}
		return r
	}, name)
	if len(name) > 80 {
		name = name[:80]
	}
	path := filepath.Join(dir, name+".log")
	body := []byte(output)
	if len(body) > maxLogBytes {
		body = body[len(body)-maxLogBytes:]
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		return "", err
	}
	return filepath.ToSlash(path), nil
}

func extractNotify(jsonArgs []byte) (pattern string, wait time.Duration) {
	wait = defaultNotifyWait
	var m map[string]any
	if err := json.Unmarshal(jsonArgs, &m); err != nil {
		return "", wait
	}
	for _, k := range []string{"notify_pattern", "notify_on_output"} {
		if s, ok := m[k].(string); ok && strings.TrimSpace(s) != "" {
			pattern = strings.TrimSpace(s)
			break
		}
	}
	if v, ok := m["yield_time_ms"]; ok {
		if n := jsonNumber(v); n > 0 {
			wait = time.Duration(n) * time.Millisecond
		}
	}
	if v, ok := m["block_until_ms"]; ok {
		if n := jsonNumber(v); n > 0 {
			wait = time.Duration(n) * time.Millisecond
		}
	}
	return pattern, wait
}

func jsonNumber(v any) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

func asMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func mergeSession(base, poll map[string]any, sid string) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for k, v := range poll {
		out[k] = v
	}
	out["session_id"] = sid
	return out
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}

func toolName(ct trpctool.CallableTool) string {
	if ct == nil {
		return ""
	}
	d := ct.Declaration()
	if d == nil {
		return ""
	}
	return d.Name
}

func clipRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

var _ trpctool.StreamableTool = (*execEnhance)(nil)
