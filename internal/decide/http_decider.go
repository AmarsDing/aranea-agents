package decide

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/outboundguard"
)

const (
	envJEVURL     = "ARANEA_JEV_URL"
	envJEVModel   = "ARANEA_JEV_MODEL"
	envJEVTimeout = "ARANEA_JEV_TIMEOUT"
	envJEVBackend = "ARANEA_JEV_BACKEND"

	defaultJEVModel   = "openjev-q4"
	defaultJEVTimeout = 8 * time.Second
	defaultJEVBackend = "openjev-http"

	// systemOneProbeTimeout bounds the native /v1/systemone attempt.
	// Recorder.fillJEV only allows 5s for the whole question. A llama.cpp
	// build without that route can sit on the request until the parent
	// deadline, which cancels the lettered-chat fallback as well.
	systemOneProbeTimeout = 1500 * time.Millisecond
)

// HTTPDecider calls an OpenJev / System-One compatible endpoint.
// Prefer POST {base}/v1/systemone when the server implements it; otherwise
// fall back to lettered POST {base}/v1/chat/completions with thinking disabled
// (llama.cpp builds without PR #29818 return 501 on /v1/systemone).
type HTTPDecider struct {
	base    string
	model   string
	backend string
	client  *http.Client
	lg      loggateway.Logger
	// systemOneProbe overrides systemOneProbeTimeout in tests.
	systemOneProbe time.Duration
}

// DeciderFromEnv builds an HTTPDecider when ARANEA_JEV_URL is set; otherwise nil.
// Safe to call at every NewRecorder: absent env keeps shadow incumbent-only.
func DeciderFromEnv(lg loggateway.Logger) Decider {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv(envJEVURL)), "/")
	if base == "" {
		return nil
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	timeout := defaultJEVTimeout
	if raw := strings.TrimSpace(os.Getenv(envJEVTimeout)); raw != "" {
		if d, err := time.ParseDuration(raw); err == nil && d > 0 {
			timeout = d
		}
	}
	model := strings.TrimSpace(os.Getenv(envJEVModel))
	if model == "" {
		model = defaultJEVModel
	}
	backend := strings.TrimSpace(os.Getenv(envJEVBackend))
	if backend == "" {
		backend = defaultJEVBackend
	}
	return &HTTPDecider{
		base:    base,
		model:   model,
		backend: backend,
		client:  outboundguard.NewClient(timeout),
		lg:      lg.With(loggateway.Domain("jev_http")),
	}
}

// Decide runs one forward pass over the question batch. Unreachable / malformed
// responses return an error so the caller keeps incumbent-only shadow records.
func (d *HTTPDecider) Decide(ctx context.Context, state map[string]any, questions []Question) (map[string]Answer, error) {
	if d == nil || d.base == "" {
		return nil, fmt.Errorf("decide: http decider not configured")
	}
	if len(questions) == 0 {
		return map[string]Answer{}, nil
	}
	out := make(map[string]Answer, len(questions))
	for _, q := range questions {
		start := time.Now()
		ans, err := d.decideOne(ctx, state, q)
		if err != nil {
			return nil, err
		}
		ans.LatencyMS = time.Since(start).Milliseconds()
		ans.Backend = d.backend
		out[q.ID] = ans
	}
	return out, nil
}

func (d *HTTPDecider) decideOne(ctx context.Context, state map[string]any, q Question) (Answer, error) {
	criteria := q.Criteria
	if len(criteria) == 0 {
		criteria = criteriaFromState(q, state)
	}
	if len(criteria) == 0 {
		return Answer{}, fmt.Errorf("decide: question %s has no option keys", q.ID)
	}
	keys := sortedKeys(criteria)
	// Prefer native systemone when this base has not already failed the probe.
	// 404/501 and a probe timeout are remembered so later questions go straight
	// to lettered chat. Each NewRecorder builds its own client, so the memory
	// is process-wide and keyed by base URL.
	if !systemOneSkipped(d.base) {
		probe := d.probeBudget()
		pctx, cancel := context.WithTimeout(ctx, probe)
		ans, err := d.systemOne(pctx, state, q, keys, criteria)
		cancel()
		if err == nil {
			return ans, nil
		}
		if isMissingRoute(err) || probeTimedOut(ctx, err) {
			markSystemOneSkipped(d.base)
		} else {
			lg := d.lg
			if lg == nil {
				lg = loggateway.NewNoop()
			}
			lg.Warn("systemone failed, trying lettered chat",
				loggateway.StepID("jev.systemone"), loggateway.Err(err))
		}
	}
	return d.letteredChat(ctx, state, q, keys, criteria)
}

func (d *HTTPDecider) probeBudget() time.Duration {
	if d != nil && d.systemOneProbe > 0 {
		return d.systemOneProbe
	}
	return systemOneProbeTimeout
}

var systemOneSkip struct {
	mu    sync.Mutex
	bases map[string]struct{}
}

func systemOneSkipped(base string) bool {
	systemOneSkip.mu.Lock()
	defer systemOneSkip.mu.Unlock()
	_, ok := systemOneSkip.bases[base]
	return ok
}

func markSystemOneSkipped(base string) {
	if base == "" {
		return
	}
	systemOneSkip.mu.Lock()
	defer systemOneSkip.mu.Unlock()
	if systemOneSkip.bases == nil {
		systemOneSkip.bases = map[string]struct{}{}
	}
	systemOneSkip.bases[base] = struct{}{}
}

func resetSystemOneSkip() {
	systemOneSkip.mu.Lock()
	systemOneSkip.bases = nil
	systemOneSkip.mu.Unlock()
}

// probeTimedOut reports a systemone attempt that exhausted its own budget
// while the caller context is still alive. Parent cancellation must not
// mark the endpoint unavailable.
func probeTimedOut(parent context.Context, err error) bool {
	if parent.Err() != nil || err == nil {
		return false
	}
	return errors.Is(err, context.DeadlineExceeded)
}

type systemOneReq struct {
	Model     string         `json:"model"`
	State     map[string]any `json:"state"`
	Questions []Question     `json:"questions"`
}

type systemOneResp struct {
	Answers map[string]struct {
		Probabilities map[string]float64 `json:"probabilities"`
		Confidence    float64            `json:"confidence"`
	} `json:"answers"`
}

func (d *HTTPDecider) systemOne(ctx context.Context, state map[string]any, q Question, keys []string, criteria map[string]string) (Answer, error) {
	qq := q
	qq.Criteria = criteria
	body, err := json.Marshal(systemOneReq{Model: d.model, State: state, Questions: []Question{qq}})
	if err != nil {
		return Answer{}, err
	}
	raw, status, err := d.post(ctx, d.base+"/v1/systemone", body)
	if err != nil {
		return Answer{}, err
	}
	if status == http.StatusNotFound || status == http.StatusNotImplemented {
		return Answer{}, missingRouteError{status: status}
	}
	if status < 200 || status >= 300 {
		return Answer{}, fmt.Errorf("decide: systemone status %d: %s", status, truncate(string(raw), 200))
	}
	var resp systemOneResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Answer{}, err
	}
	slot, ok := resp.Answers[q.ID]
	if !ok || len(slot.Probabilities) == 0 {
		return Answer{}, fmt.Errorf("decide: systemone missing answer for %s", q.ID)
	}
	conf := slot.Confidence
	if conf <= 0 {
		conf = maxProb(slot.Probabilities)
	}
	return Answer{Probabilities: slot.Probabilities, Confidence: conf}, nil
}

type chatReq struct {
	Model              string              `json:"model"`
	Messages           []map[string]string `json:"messages"`
	MaxTokens          int                 `json:"max_tokens"`
	Temperature        float64             `json:"temperature"`
	ChatTemplateKwargs map[string]any      `json:"chat_template_kwargs"`
}

type chatResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (d *HTTPDecider) letteredChat(ctx context.Context, state map[string]any, q Question, keys []string, criteria map[string]string) (Answer, error) {
	prompt := buildLetteredPrompt(state, q, keys, criteria)
	body, err := json.Marshal(chatReq{
		Model:              d.model,
		Messages:           []map[string]string{{"role": "user", "content": prompt}},
		MaxTokens:          8,
		Temperature:        0,
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	})
	if err != nil {
		return Answer{}, err
	}
	raw, status, err := d.post(ctx, d.base+"/v1/chat/completions", body)
	if err != nil {
		return Answer{}, err
	}
	if status < 200 || status >= 300 {
		return Answer{}, fmt.Errorf("decide: chat status %d: %s", status, truncate(string(raw), 200))
	}
	var resp chatResp
	if err := json.Unmarshal(raw, &resp); err != nil {
		return Answer{}, err
	}
	if len(resp.Choices) == 0 {
		return Answer{}, fmt.Errorf("decide: empty chat choices")
	}
	letter := extractLetter(resp.Choices[0].Message.Content)
	idx := letterIndex(letter)
	if idx < 0 || idx >= len(keys) {
		return Answer{}, fmt.Errorf("decide: cannot map letter %q to options", letter)
	}
	probs := softOneHot(keys, idx)
	return Answer{Probabilities: probs, Confidence: probs[keys[idx]]}, nil
}

func (d *HTTPDecider) post(ctx context.Context, url string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return raw, resp.StatusCode, nil
}

func buildLetteredPrompt(state map[string]any, q Question, keys []string, criteria map[string]string) string {
	var b strings.Builder
	b.WriteString("State:\n")
	for _, slot := range q.Slots {
		if v, ok := state[slot]; ok {
			fmt.Fprintf(&b, "%s: %v\n", slot, v)
		}
	}
	if len(q.Slots) == 0 {
		for k, v := range state {
			fmt.Fprintf(&b, "%s: %v\n", k, v)
		}
	}
	b.WriteString("\nQuestion: ")
	b.WriteString(strings.TrimSpace(q.Instructions))
	b.WriteString("\nOptions:\n")
	for i, key := range keys {
		fmt.Fprintf(&b, "[%c] %s", 'A'+rune(i), key)
		if c := strings.TrimSpace(criteria[key]); c != "" {
			fmt.Fprintf(&b, " — %s", c)
		}
		b.WriteByte('\n')
	}
	b.WriteString("\nAnswer with the letter of the best option only.")
	return b.String()
}

func criteriaFromState(q Question, state map[string]any) map[string]string {
	out := map[string]string{}
	add := func(keys []string) {
		for _, k := range keys {
			k = strings.TrimSpace(k)
			if k != "" {
				out[k] = k
			}
		}
	}
	switch q.ID {
	case "skill_intent":
		add(stringSlice(state["skill_leaves"]))
		out["none"] = "no skill fits"
	case "mcp_server_choice":
		add(stringSlice(state["mcp_servers"]))
		out["none"] = "no server fits"
		out["mount_all"] = "mount the filtered server set"
	case "tool_relevance":
		out["needed"] = "tool is needed"
		out["not_needed"] = "tool is not needed"
	}
	return out
}

func stringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		out := make([]string, 0, len(t))
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func softOneHot(keys []string, win int) map[string]float64 {
	n := len(keys)
	out := make(map[string]float64, n)
	if n == 0 {
		return out
	}
	rest := 0.05
	if n == 1 {
		out[keys[0]] = 1
		return out
	}
	per := rest / float64(n-1)
	for i, k := range keys {
		if i == win {
			out[k] = 1 - rest
		} else {
			out[k] = per
		}
	}
	return out
}

func extractLetter(content string) string {
	for _, r := range strings.TrimSpace(content) {
		if unicode.IsLetter(r) {
			return strings.ToUpper(string(r))
		}
	}
	return ""
}

func letterIndex(letter string) int {
	if len(letter) != 1 {
		return -1
	}
	c := letter[0]
	if c < 'A' || c > 'Z' {
		return -1
	}
	return int(c - 'A')
}

func maxProb(m map[string]float64) float64 {
	best := 0.0
	for _, v := range m {
		if v > best {
			best = v
		}
	}
	return best
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

type missingRouteError struct{ status int }

func (e missingRouteError) Error() string {
	return fmt.Sprintf("decide: systemone unavailable (%d)", e.status)
}

func isMissingRoute(err error) bool {
	_, ok := err.(missingRouteError)
	return ok
}
