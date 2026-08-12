package computeruse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

// OmniParserClient 实现 bizcu.VisionParser：OmniParser V2 omniparserserver 的 HTTP 客户端。
// 健康检查带 TTL 缓存 + Parse 失败主动降级标记，避免每次 Act 都打挂掉的服务。
type OmniParserClient struct {
	baseURL string
	hc      *http.Client
	lg      loggateway.Logger

	// probeTTL 健康状态缓存时长（测试可注入短值）。
	probeTTL time.Duration

	mu         sync.Mutex
	healthy    bool
	probedAt   time.Time // 上次真探测时间；零值表示从未探测
	probedOnce bool
}

// NewOmniParserClient 构造客户端。baseURL 形如 http://127.0.0.1:8100。
func NewOmniParserClient(baseURL string, lg loggateway.Logger) *OmniParserClient {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &OmniParserClient{
		baseURL:  baseURL,
		hc:       &http.Client{Timeout: 30 * time.Second},
		lg:       lg,
		probeTTL: 30 * time.Second,
	}
}

// omniparserParsedItem omniparserserver /parse/ 响应元素。
type omniparserParsedItem struct {
	Type          string     `json:"type"`
	Content       string     `json:"content"`
	BBox          [4]float64 `json:"bbox"` // 归一化 [x1,y1,x2,y2]
	Interactivity bool       `json:"interactivity"`
	Source        string     `json:"source"`
}

// Available 健康检查：TTL 内返回缓存；过期真探测 GET /probe/（2s 超时）。
func (c *OmniParserClient) Available(ctx context.Context) bool {
	c.mu.Lock()
	if c.probedOnce && time.Since(c.probedAt) < c.probeTTL {
		ok := c.healthy
		c.mu.Unlock()
		return ok
	}
	c.mu.Unlock()

	ok := c.probe(ctx)

	c.mu.Lock()
	c.healthy = ok
	c.probedAt = time.Now()
	c.probedOnce = true
	c.mu.Unlock()
	return ok
}

func (c *OmniParserClient) probe(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/probe/", nil)
	if err != nil {
		return false
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// markUnhealthy Parse 失败后主动失效健康缓存（降级标记）。
func (c *OmniParserClient) markUnhealthy() {
	c.mu.Lock()
	c.healthy = false
	c.probedAt = time.Now()
	c.probedOnce = true
	c.mu.Unlock()
}

// Parse POST /parse/ {base64_image} → parsed_content_list → UIElement[]。
// 归一化 bbox 按图像尺寸换算为物理像素；Source 统一 vision。
func (c *OmniParserClient) Parse(ctx context.Context, img bizcu.Image) ([]bizcu.UIElement, error) {
	body, err := json.Marshal(map[string]any{
		"base64_image": base64.StdEncoding.EncodeToString(img.PNG),
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/parse/", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.hc.Do(req)
	if err != nil {
		c.markUnhealthy()
		c.lg.Warn("omniparser parse request failed", loggateway.Err(err))
		return nil, fmt.Errorf("computeruse: omniparser 请求失败: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		c.markUnhealthy()
		return nil, fmt.Errorf("computeruse: omniparser 返回 %d", resp.StatusCode)
	}

	var out struct {
		ParsedContentList []omniparserParsedItem `json:"parsed_content_list"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("computeruse: omniparser 响应解码失败: %w", err)
	}

	els := make([]bizcu.UIElement, 0, len(out.ParsedContentList))
	for _, it := range out.ParsedContentList {
		x1 := int(it.BBox[0] * float64(img.Width))
		y1 := int(it.BBox[1] * float64(img.Height))
		x2 := int(it.BBox[2] * float64(img.Width))
		y2 := int(it.BBox[3] * float64(img.Height))
		els = append(els, bizcu.UIElement{
			Type:          it.Type,
			Name:          it.Content,
			BBox:          bizcu.Rect{X: x1, Y: y1, W: x2 - x1, H: y2 - y1},
			Interactivity: it.Interactivity,
			Source:        "vision",
			Enabled:       true,
		})
	}
	return els, nil
}
