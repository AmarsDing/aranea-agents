package fleet

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// 样本取自 2026-11-21 本机各工具的真实输出。
func TestParseModels(t *testing.T) {
	cases := map[string]struct {
		out  string
		want []string
	}{
		"cursor": {"Available models\n\nauto - Auto (current, default)\ngpt-5.3-codex - Codex 5.3\n", []string{"auto", "gpt-5.3-codex"}},
		"grok":   {"You are logged in with grok.com.\n\nDefault model: grok-4.7\n\nAvailable models:\n  * grok-4.7 (default)\n", []string{"grok-4.7"}},
		"qoder":  {"MODEL\nAuto\nUltimate\nQwen4-Max\n", []string{"Auto", "Ultimate", "Qwen4-Max"}},
		"codex": {`{"models":[{"slug":"gpt-reserve","visibility":"hide"},{"slug":"gpt-6-luna","visibility":"list"},{"slug":"gpt-5.6-terra"}]}`,
			[]string{"gpt-6-luna", "gpt-5.6-terra"}},
		"codebuddy": {"  --model <model>  Model for the current session. Currently supported: (hy4std, glm-5.3, kimi-k3.1-pro)\n",
			[]string{"hy4std", "glm-5.3", "kimi-k3.1-pro"}},
		"error": {"internal_error: 无法启动主任务。请稍后重试。\n", nil},
		"doubao": {"[cli-chat] {\"level\":\"info\"}\n{\"type\":\"result\",\"result\":{\"type\":\"model_list\",\"models\":[{\"id\":\"9\",\"name\":\"Auto\",\"is_default\":true,\"efforts\":[\"low\",\"medium\",\"high\"],\"default_effort\":\"low\"},{\"id\":\"seed-lite-7b\",\"name\":\"豆包 2.1 Lite\"}]}}\n",
			[]string{"9", "seed-lite-7b"}},
	}
	for name, c := range cases {
		if got := modelIDs(parseModels(c.out)); !slices.Equal(got, c.want) {
			t.Errorf("%s: got %v want %v", name, got, c.want)
		}
	}
}

func modelIDs(ms []ToolModel) []string {
	var ids []string
	for _, m := range ms {
		ids = append(ids, m.ID)
	}
	return ids
}

// 名称、默认、各型号支持的强度要一起带出来（模型菜单靠它们显示版本与能力）。
func TestParseModelDetails(t *testing.T) {
	codex := `{"models":[{"slug":"gpt-6-luna","display_name":"GPT-6 Luna","description":"Fast","default_reasoning_level":"medium","supported_reasoning_levels":[{"effort":"low"},{"effort":"xhigh"}]}]}`
	m := parseModels(codex)[0]
	if m.Name != "GPT-6 Luna" || m.Desc != "Fast" || m.DefaultEffort != "medium" || !slices.Equal(m.Efforts, []string{"low", "xhigh"}) {
		t.Fatalf("codex: %+v", m)
	}
	doubao := `{"result":{"models":[{"id":"9","name":"Auto","is_default":true,"efforts":["low","high"],"default_effort":"low"}]}}`
	if m := parseModels(doubao)[0]; m.Name != "Auto" || !m.Default || m.DefaultEffort != "low" || len(m.Efforts) != 2 {
		t.Fatalf("doubao: %+v", m)
	}
	if m := parseModels("auto - Auto (current, default)\ngpt-5 - GPT 5\n"); m[0].Name != "Auto" || !m[0].Default || m[1].Default {
		t.Fatalf("cursor: %+v", m)
	}
}

func TestWithModelAndFailLine(t *testing.T) {
	p := AgentPreset{ModelFlag: "-m"}
	p.EffortArgs = []string{"-c", "model_reasoning_effort={effort}"}
	if got := withModel(p, "gpt-6-luna", "xhigh", []string{"--x"}); !slices.Equal(got, []string{"-m", "gpt-6-luna", "-c", "model_reasoning_effort=xhigh", "--x"}) {
		t.Fatalf("withModel: %v", got)
	}
	if got := withModel(p, "a", "", []string{"-m", "b"}); !slices.Equal(got, []string{"-m", "b"}) {
		t.Fatal("a model already in args must win")
	}
	if modelRe.MatchString("gpt&calc") || !modelRe.MatchString("gpt-5[reasoning=high]") {
		t.Fatal("model name validation")
	}
	out := "user\nwarning: tier\nERROR: The 'gpt-6.1-sol' model is not supported when using Codex with a ChatGPT account.\n"
	if got := failLine(out, "exit status 1"); !strings.HasPrefix(got, "ERROR: The 'gpt-6.1-sol'") {
		t.Fatalf("failLine: %q", got)
	}
	if got := failLine("nothing useful", "exit status 1"); got != "exit status 1" {
		t.Fatalf("fallback: %q", got)
	}
}

// Gemini 用 Key 时换一个只选 API Key 的配置目录（盖过用户设置里失效的 Google 登录，用户配置不动）。
func TestGeminiKeyEnvOverridesAuthType(t *testing.T) {
	t.Setenv("ARANEA_FLEET_CLI_ROOT", t.TempDir())
	p := AgentPreset{ID: "gemini"}
	applyDefaultAuth(&p)
	env := keyEnvFor(p, "k")
	if len(env) != 3 || env[0] != "GEMINI_API_KEY=k" || !strings.HasPrefix(env[1], "GEMINI_CLI_HOME=") || env[2] != "GEMINI_CLI_TRUST_WORKSPACE=true" {
		t.Fatalf("env: %v", env)
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimPrefix(env[1], "GEMINI_CLI_HOME="), ".gemini", "settings.json"))
	if err != nil || !strings.Contains(string(data), `"selectedType":"gemini-api-key"`) || !p.KeyOnly {
		t.Fatalf("settings file: %s err=%v keyOnly=%v", data, err, p.KeyOnly)
	}
}

// Claude 存了 Key 时走 Anthropic 的模型列表接口：带上 Key、自动翻页。
func TestAnthropicModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" || r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("after_id") == "" {
			_, _ = w.Write([]byte(`{"data":[{"id":"claude-opus-5-5","display_name":"Claude Opus 5.5","type":"model"}],"has_more":true,"first_id":"claude-opus-5-5","last_id":"claude-opus-5-5"}`))
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"claude-sonnet-5-5","display_name":"Claude Sonnet 5.5","type":"model"}],"has_more":false,"first_id":"claude-sonnet-5-5","last_id":"claude-sonnet-5-5"}`))
	}))
	defer srv.Close()
	got, err := anthropicModels(context.Background(), "test-key", srv.URL)
	if err != nil || !slices.Equal(modelIDs(got), []string{"claude-opus-5-5", "claude-sonnet-5-5"}) || got[0].Name != "Claude Opus 5.5" {
		t.Fatalf("models=%v err=%v", got, err)
	}
}

// Gemini：Key 放请求头、翻页、只留能对话的模型；Key 无效时带出 Google 的原话。
func TestGeminiModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("x-goog-api-key") != "good" || r.URL.Query().Get("key") != "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key."}}`))
			return
		}
		if r.URL.Query().Get("pageToken") == "" {
			_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-3-pro","supportedGenerationMethods":["generateContent"]},{"name":"models/text-embedding-9","supportedGenerationMethods":["embedContent"]}],"nextPageToken":"p2"}`))
			return
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"models/gemini-3-flash","supportedGenerationMethods":["generateContent","countTokens"]}]}`))
	}))
	defer srv.Close()
	got, err := geminiModels(context.Background(), "good", srv.URL)
	if err != nil || !slices.Equal(modelIDs(got), []string{"gemini-3-pro", "gemini-3-flash"}) {
		t.Fatalf("models=%v err=%v", got, err)
	}
	if _, err := geminiModels(context.Background(), "bad", srv.URL); err == nil || !strings.Contains(err.Error(), "API key not valid") {
		t.Fatalf("bad key err=%v", err)
	}
}

func TestVerifyKeyByModels(t *testing.T) {
	cases := map[string]struct {
		models []ToolModel
		err    error
		ok     bool
		prefix string
	}{
		"valid":   {[]ToolModel{{ID: "m1"}}, nil, true, "能用"},
		"invalid": {nil, errors.New("HTTP 400：API key not valid"), false, "要登录"},
		"quota":   {nil, errors.New("HTTP 429：Resource has been exhausted (e.g. check quota)"), false, "模型服务拒绝"},
		"network": {nil, errors.New("dial tcp: i/o timeout"), false, "连不上厂商接口"},
	}
	for name, c := range cases {
		f := func(context.Context, string, string) ([]ToolModel, error) { return c.models, c.err }
		res := verifyKeyByModels(context.Background(), "k", f, &installJob{})
		if res.OK != c.ok || !strings.HasPrefix(res.Verdict, c.prefix) {
			t.Errorf("%s: %+v", name, res)
		}
	}
}

func TestDeepseekModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/models" || r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":{"message":"Authentication Fails, Your api key: ****bad is invalid","type":"authentication_error"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek-v4","object":"model"},{"id":"deepseek-v4-reasoner","object":"model"}]}`))
	}))
	defer srv.Close()
	got, err := deepseekModels(context.Background(), "good", srv.URL)
	if err != nil || !slices.Equal(modelIDs(got), []string{"deepseek-v4", "deepseek-v4-reasoner"}) {
		t.Fatalf("models=%v err=%v", got, err)
	}
	_, err = deepseekModels(context.Background(), "bad", srv.URL)
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("bad key err=%v", err)
	}
	if res := verifyKeyByModels(context.Background(), "bad", func(ctx context.Context, k, _ string) ([]ToolModel, error) {
		return deepseekModels(ctx, k, srv.URL)
	}, &installJob{}); res.OK || !strings.HasPrefix(res.Verdict, "要登录") {
		t.Fatalf("bad key verdict: %+v", res)
	}
}
