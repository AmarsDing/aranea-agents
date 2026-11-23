package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"aranea-agents/pkg/apierror"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// 选模型与思考强度：工具设置里记模型和强度，启动终端、连通测试、派活都带上该工具的参数；
// 可选的模型（名称、版本、支持的强度）由厂商接口或工具自己的列表命令给出，各家输出格式见 parseModels。

// ToolModel 是一个可选模型。Efforts 为空时用工具级的强度档位。
type ToolModel struct {
	ID            string   `json:"id"`
	Name          string   `json:"name,omitempty"`
	Desc          string   `json:"desc,omitempty"`
	Efforts       []string `json:"efforts,omitempty"`
	DefaultEffort string   `json:"default_effort,omitempty"`
	Default       bool     `json:"default,omitempty"`
}

// modelRe 只放行模型名常见字符；参数经 cmd.exe 传给 .cmd 垫片，不能带 shell 元字符。
// Cursor 的带参模型形如 gpt-5[reasoning=high]。
var (
	modelRe  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/\-\[\]=,]{0,99}$`)
	effortRe = regexp.MustCompile(`^[a-z]{1,16}$`)
)

// withModel 把模型与思考强度参数放在 args 前面；附加参数里已经写了同一参数时不重复加。
func withModel(p AgentPreset, model, effort string, args []string) []string {
	var pre []string
	if model != "" && p.ModelFlag != "" && !slices.Contains(args, p.ModelFlag) {
		pre = append(pre, p.ModelFlag, model)
	}
	if effort != "" && len(p.EffortArgs) > 0 && !slices.Contains(args, p.EffortArgs[0]) {
		for _, a := range p.EffortArgs {
			pre = append(pre, strings.ReplaceAll(a, "{effort}", effort))
		}
	}
	return append(pre, args...)
}

// 模型列表从哪来：厂商接口（用已存的 Key）/ 工具自己的列表命令 / 内置的当前在售型号。
const (
	ModelsFromAPI    = "api"
	ModelsFromTool   = "tool"
	ModelsFromStatic = "static"
)

// ListToolModels 实时取工具可用的模型：存了 Key 且厂商有列表接口的直接问接口，其次跑工具自己的列表命令，
// 都没有就给内置的当前在售型号。
func (u *Usecase) ListToolModels(id string) (models []ToolModel, source string, err error) {
	p, ok := findPreset(id)
	if !ok || p.ID == "shell" {
		return nil, "", apierror.BadRequest(domain, "unknown agent: "+id)
	}
	if p.ModelFlag == "" {
		return nil, "", apierror.FailedPrecondition(domain, p.Label+" 不支持在这里选模型")
	}
	ctx, cancel := context.WithTimeout(u.ctx, 45*time.Second)
	defer cancel()
	if f := defaultAuth[p.ID].keyModels; f != nil {
		if key := u.toolKey(p); key != "" {
			if models, err := f(ctx, key, ""); err == nil && len(models) > 0 {
				return models, ModelsFromAPI, nil
			}
		}
	}
	if len(p.ModelsArgs) == 0 || !p.Available {
		return p.Models, ModelsFromStatic, nil
	}
	var buf strings.Builder
	err = runTool(ctx, p, p.ModelsArgs, os.TempDir(), append(u.terminalEnv("toolmodels", ""), u.toolKeyEnv(p)...), &buf)
	out := StripANSI(buf.String())
	if models := parseModels(out); len(models) > 0 {
		return models, ModelsFromTool, nil
	}
	// 有的工具出错也返回 0（豆包 model list 报 internal_error），只要没列出模型就把它的原话带回去
	if why := strings.TrimSpace(lastLines(out, 1)); err != nil || why != "" {
		if why == "" {
			why = err.Error()
		}
		return p.Models, ModelsFromStatic, apierror.FailedPrecondition(domain, p.Label+" 没列出模型："+failLine(out, why))
	}
	return p.Models, ModelsFromStatic, nil
}

// keyModelsFunc 用 API Key 调厂商的模型列表接口；base 非空时换接口地址（测试用）。
type keyModelsFunc func(ctx context.Context, key, base string) ([]ToolModel, error)

// anthropicModels 用 API Key 调 Anthropic 的模型列表接口（官方 SDK，自动翻页），带名称与支持的思考强度。
func anthropicModels(ctx context.Context, key, base string) ([]ToolModel, error) {
	opts := []option.RequestOption{option.WithAPIKey(key)}
	if base != "" {
		opts = append(opts, option.WithBaseURL(base))
	}
	c := anthropic.NewClient(opts...)
	var out []ToolModel
	it := c.Models.ListAutoPaging(ctx, anthropic.ModelListParams{Limit: anthropic.Int(1000)})
	for it.Next() {
		m := it.Current()
		e := m.Capabilities.Effort
		var efforts []string
		for _, lv := range []struct {
			name string
			ok   bool
		}{{"low", e.Low.Supported}, {"medium", e.Medium.Supported}, {"high", e.High.Supported}, {"xhigh", e.Xhigh.Supported}, {"max", e.Max.Supported}} {
			if e.Supported && lv.ok {
				efforts = append(efforts, lv.name)
			}
		}
		out = append(out, ToolModel{ID: m.ID, Name: m.DisplayName, Efforts: efforts})
	}
	return out, it.Err()
}

const deepseekAPI = "https://api.deepseek.com"

// deepseekModels 用 API Key 调 DeepSeek 的模型列表（OpenAI 兼容 GET /models，Key 走 Authorization 头）。
func deepseekModels(ctx context.Context, key, base string) ([]ToolModel, error) {
	if base == "" {
		base = deepseekAPI
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	derr := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body)
	if resp.StatusCode != http.StatusOK {
		if body.Error != nil {
			return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, body.Error.Message)
		}
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	if derr != nil {
		return nil, derr
	}
	out := make([]ToolModel, 0, len(body.Data))
	for _, m := range body.Data {
		out = append(out, ToolModel{ID: m.ID})
	}
	return out, nil
}

const geminiAPI = "https://generativelanguage.googleapis.com"

// geminiModels 用 API Key 调 Gemini API 的模型列表（Key 放请求头，不进 URL），只留能对话的模型。
func geminiModels(ctx context.Context, key, base string) ([]ToolModel, error) {
	if base == "" {
		base = geminiAPI
	}
	var out []ToolModel
	token := ""
	for page := 0; page < 20; page++ {
		u := base + "/v1beta/models?pageSize=1000"
		if token != "" {
			u += "&pageToken=" + url.QueryEscape(token)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("x-goog-api-key", key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		var body struct {
			Models []struct {
				Name        string   `json:"name"`
				DisplayName string   `json:"displayName"`
				Description string   `json:"description"`
				Methods     []string `json:"supportedGenerationMethods"`
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"`
			Error         *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		derr := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			if body.Error != nil {
				return nil, fmt.Errorf("HTTP %d：%s", resp.StatusCode, body.Error.Message)
			}
			return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		if derr != nil {
			return nil, derr
		}
		for _, m := range body.Models {
			if slices.Contains(m.Methods, "generateContent") {
				out = append(out, ToolModel{ID: strings.TrimPrefix(m.Name, "models/"), Name: m.DisplayName, Desc: m.Description})
			}
		}
		if token = body.NextPageToken; token == "" {
			break
		}
	}
	return out, nil
}

// verifyKeyByModels 拿候选 Key 调模型列表接口：取到模型 = Key 有效；401/403/400 = 无效；429 = 有效但额度受限。
func verifyKeyByModels(parent context.Context, key string, f keyModelsFunc, j *installJob) TestResult {
	j.logf("调厂商的模型列表接口验证 Key（不消耗对话额度，最长 30 秒）")
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	start := time.Now()
	models, err := f(ctx, key, "")
	res := TestResult{DurationMS: time.Since(start).Milliseconds()}
	if err == nil && len(models) > 0 {
		names := make([]string, 0, 6)
		for _, m := range models[:min(len(models), 6)] {
			names = append(names, m.ID)
		}
		head := strings.Join(names, ", ")
		j.logf("✔ 取到 %d 个可用模型：%s", len(models), head)
		res.OK, res.Verdict, res.Reply = true, fmt.Sprintf("能用（取到 %d 个可用模型）", len(models)), head
		return res
	}
	msg := "接口没返回任何模型"
	if err != nil {
		msg = err.Error()
	}
	if r := []rune(msg); len(r) > 300 {
		msg = string(r[:300]) + "…"
	}
	j.logf("✖ %s", msg)
	res.Reply = msg
	low := strings.ToLower(msg)
	switch {
	case quotaHit(low):
		res.Verdict = "模型服务拒绝：额度用完或被限流"
	case containsAny(low, "api key not valid", "api_key_invalid", "invalid x-api-key", "authentication_error", "permission_denied", "401", "403", "http 400"):
		res.Verdict = "要登录：Key 无效或没有权限"
	default:
		res.Verdict = "连不上厂商接口（检查网络或代理）：" + msg
	}
	return res
}

var supportedListRe = regexp.MustCompile(`(?i)supported:\s*\(([^)]*)\)`)

// catalogModel 兼容两家 JSON：Codex `debug models`（slug / display_name / supported_reasoning_levels）
// 与豆包 `model list --json`（result.models 里的 id / name / efforts）。
type catalogModel struct {
	Slug, ID, Name, Description, Visibility string
	DisplayName                             string   `json:"display_name"`
	DefaultReasoning                        string   `json:"default_reasoning_level"`
	DefaultEffort                           string   `json:"default_effort"`
	IsDefault                               bool     `json:"is_default"`
	Efforts                                 []string `json:"efforts"`
	Levels                                  []struct {
		Effort string `json:"effort"`
	} `json:"supported_reasoning_levels"`
}

func catalogFromJSON(s string) []catalogModel {
	var cat struct {
		Models []catalogModel `json:"models"`
		Result struct {
			Models []catalogModel `json:"models"`
		} `json:"result"`
	}
	if json.Unmarshal([]byte(s), &cat) != nil {
		return nil
	}
	return append(cat.Models, cat.Result.Models...)
}

// parseModels 认四种输出：JSON 目录（Codex、豆包；前面可能夹着日志行）、「supported: (a, b)」（CodeBuddy --help）、
// 每行一个模型（Cursor「id - 名称」、Grok「* id (default)」、Qoder 一行一个名称）。
func parseModels(out string) []ToolModel {
	var models []ToolModel
	add := func(m ToolModel) {
		m.ID = strings.TrimSpace(m.ID)
		if modelRe.MatchString(m.ID) && len(models) < 1000 && !slices.ContainsFunc(models, func(x ToolModel) bool { return x.ID == m.ID }) {
			models = append(models, m)
		}
	}
	var cat []catalogModel
	if i := strings.Index(out, "{"); i >= 0 {
		cat = catalogFromJSON(out[i:])
	}
	for _, line := range strings.Split(out, "\n") {
		if l := strings.TrimSpace(line); len(cat) == 0 && strings.HasPrefix(l, "{") {
			cat = catalogFromJSON(l)
		}
	}
	if len(cat) > 0 {
		for _, m := range cat {
			if m.Visibility == "hide" {
				continue
			}
			tm := ToolModel{ID: m.Slug + m.ID, Name: m.DisplayName + m.Name, Desc: m.Description, Efforts: m.Efforts,
				DefaultEffort: m.DefaultReasoning + m.DefaultEffort, Default: m.IsDefault}
			for _, lv := range m.Levels {
				tm.Efforts = append(tm.Efforts, lv.Effort)
			}
			add(tm)
		}
		return models
	}
	if m := supportedListRe.FindStringSubmatch(out); m != nil {
		for _, s := range strings.Split(m[1], ",") {
			add(ToolModel{ID: s})
		}
		return models
	}
	for _, line := range strings.Split(out, "\n") {
		l := strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "*-•>"))
		low := strings.ToLower(l)
		if l == "" || strings.HasSuffix(l, ":") || strings.HasSuffix(l, ".") || strings.Contains(l, ": ") ||
			low == "model" || low == "models" || low == "available models" {
			continue
		}
		tm := ToolModel{Default: strings.Contains(low, "default")}
		if i := strings.Index(l, " - "); i > 0 {
			tm.Name = strings.TrimSpace(strings.Split(l[i+3:], " (")[0])
			l = l[:i]
		}
		tm.ID = strings.Fields(l)[0]
		add(tm)
	}
	return models
}
