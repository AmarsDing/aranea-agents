// Package apiclient 是围绕 Aranea REST API（/api/v1/*）的轻量 HTTP 封装。
// 各 Cobra 子命令与 console launcher 均经此客户端发请求，以保持鉴权、
// 输出格式默认与错误报告一致。
package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	cliconfig "arenea/backend/cmd/aranea/cli/config"
)

// GlobalContext 保存各子命令共享的标志与已解析配置。root.New() 在 Cobra
// 树中挂入单例指针，各命令惰性读取。
type GlobalContext struct {
	BaseURL string
	Token   string
	Output  string
	Quiet   bool
	Yes     bool
	Profile string
	NoColor bool
	Timeout time.Duration

	// 自 ~/.aranea/config.toml 与环境变量合并后的已解析配置，由 Resolve() 填充。
	Config *cliconfig.Config

	resolved bool
	client   *Client
}

// NewGlobalContext 返回零值上下文。调用方应自 Cobra 标志填充公开字段，
// 并在 PersistentPreRunE 中调用 Resolve()。
func NewGlobalContext() *GlobalContext {
	return &GlobalContext{}
}

// Resolve 从磁盘加载配置（若存在），并按 前端/25 cli.md §10 规定的优先级
// 合并环境变量与 CLI 标志：
//
//	flag > 环境变量 > 活动 profile > 默认值。
func (g *GlobalContext) Resolve() error {
	if g.resolved {
		return nil
	}
	cfg, err := cliconfig.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	profile := g.Profile
	if profile == "" {
		profile = os.Getenv("ARANEA_PROFILE")
	}
	if profile == "" {
		profile = cfg.Default
	}
	if profile == "" {
		profile = "default"
	}
	prof := cfg.Profile(profile)

	if g.BaseURL == "" {
		if env := os.Getenv("ARANEA_BASE_URL"); env != "" {
			g.BaseURL = env
		} else if prof != nil && prof.BaseURL != "" {
			g.BaseURL = prof.BaseURL
		} else {
			g.BaseURL = "http://127.0.0.1:8787"
		}
	}
	if g.Token == "" {
		if env := os.Getenv("ARANEA_TOKEN"); env != "" {
			g.Token = env
		} else if prof != nil {
			g.Token = prof.Token
		}
	}
	if g.Output == "" {
		if env := os.Getenv("ARANEA_OUTPUT"); env != "" {
			g.Output = env
		} else if prof != nil && prof.Output != "" {
			g.Output = prof.Output
		} else {
			g.Output = "text"
		}
	}
	if g.Timeout == 0 {
		g.Timeout = 60 * time.Second
	}

	g.Config = cfg
	g.client = newClient(g.BaseURL, g.Token, g.Timeout)
	g.resolved = true
	return nil
}

// Client 返回惰性构造的 HTTP 客户端。须先调用 Resolve()；实际中通过
// 根命令的 PersistentPreRunE 自动完成。
func (g *GlobalContext) Client() *Client {
	if g.client == nil {
		// 防御性：若未经 Cobra 框架调用（测试、console launcher）仍构默认客户端。
		_ = g.Resolve()
	}
	return g.client
}

// Client 是对 *http.Client 的薄封装，注入基址、bearer token 与若干辅助方法。
// 故意不生成强类型绑定：各子命令自行标注期望的响应形态。
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

func newClient(baseURL, token string, timeout time.Duration) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		http:    &http.Client{Timeout: timeout},
	}
}

// BaseURL 返回解析后的后端根址，供 console 等需手写流式 URL 的场景使用。
func (c *Client) BaseURL() string { return c.baseURL }

// Token 返回解析后的 bearer token，供 console 流式辅助函数附加 Authorization 头。
func (c *Client) Token() string { return c.token }

// Get 对 API 执行 GET 并将 JSON 响应解码到 out。out 为 nil 时忽略 body。
func (c *Client) Get(ctx context.Context, path string, query url.Values, out any) error {
	return c.do(ctx, http.MethodGet, path, query, nil, "", out)
}

// Post 执行带 JSON body 的 POST 请求。
func (c *Client) Post(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPost, path, nil, body, "application/json", out)
}

// Put 执行带 JSON body 的 PUT 请求。
func (c *Client) Put(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPut, path, nil, body, "application/json", out)
}

// Patch 执行带 JSON body 的 PATCH 请求。
func (c *Client) Patch(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodPatch, path, nil, body, "application/json", out)
}

// Delete 执行 DELETE 请求，可选带 JSON body。
func (c *Client) Delete(ctx context.Context, path string, body any, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, body, "application/json", out)
}

// PostMultipart 提交 multipart/form-data。第一个参数为文件字段名，第二个为
// 向服务端报告的文件名。
func (c *Client) PostMultipart(ctx context.Context, path, fieldName, fileName string, body io.Reader, extraFields map[string]string, out any) error {
	pipeR, pipeW := io.Pipe()
	mw := multipart.NewWriter(pipeW)

	go func() {
		defer pipeW.Close()
		defer mw.Close()
		for k, v := range extraFields {
			if err := mw.WriteField(k, v); err != nil {
				_ = pipeW.CloseWithError(err)
				return
			}
		}
		fw, err := mw.CreateFormFile(fieldName, fileName)
		if err != nil {
			_ = pipeW.CloseWithError(err)
			return
		}
		if _, err := io.Copy(fw, body); err != nil {
			_ = pipeW.CloseWithError(err)
			return
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(path, nil), pipeR)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	c.injectAuth(req)
	return c.execute(req, out)
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, body any, contentType string, out any) error {
	var reader io.Reader
	if body != nil {
		buf, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request body: %w", err)
		}
		reader = bytes.NewReader(buf)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.url(path, query), reader)
	if err != nil {
		return err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	req.Header.Set("Accept", "application/json")
	c.injectAuth(req)
	return c.execute(req, out)
}

func (c *Client) execute(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("call %s %s: %w", req.Method, req.URL.Path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return parseAPIError(resp.StatusCode, raw)
	}
	if out == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	dec := json.NewDecoder(resp.Body)
	if err := dec.Decode(out); err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}

func (c *Client) injectAuth(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	req.Header.Set("User-Agent", "aranea-cli")
}

func (c *Client) url(path string, query url.Values) string {
	full := c.baseURL + path
	if query != nil && len(query) > 0 {
		full += "?" + query.Encode()
	}
	return full
}

// APIError 是后端为任意 4xx/5xx 返回的结构化错误类型。实现 `error` 接口，
// 以便上游用 errors.As / errors.Is 比较。
type APIError struct {
	Status  int
	Code    string `json:"code"`
	Message string `json:"message"`
	Detail  any    `json:"detail,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("api %d %s: %s", e.Status, e.Code, e.Message)
	}
	return fmt.Sprintf("api %d: %s", e.Status, e.Message)
}

func parseAPIError(status int, raw []byte) error {
	apiErr := &APIError{Status: status}
	if len(raw) == 0 {
		apiErr.Message = http.StatusText(status)
		return apiErr
	}
	var wrapper struct {
		Error *APIError `json:"error"`
	}
	if err := json.Unmarshal(raw, &wrapper); err == nil && wrapper.Error != nil {
		wrapper.Error.Status = status
		return wrapper.Error
	}
	if err := json.Unmarshal(raw, apiErr); err == nil && (apiErr.Code != "" || apiErr.Message != "") {
		return apiErr
	}
	apiErr.Message = strings.TrimSpace(string(raw))
	if apiErr.Message == "" {
		apiErr.Message = http.StatusText(status)
	}
	return apiErr
}
