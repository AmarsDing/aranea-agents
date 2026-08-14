package computeruse

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

// SpecialistGrounder 专用 GUI grounding HTTP 客户端（M3.2）。
// POST {base}/ground  {image_base64,target,width,height} → {x,y}；负坐标=无匹配。
// URL 空则不应被装配（Deps.Specialist=nil）。
type SpecialistGrounder struct {
	baseURL string
	hc      *http.Client
	lg      loggateway.Logger
}

// NewSpecialistGrounder 构造；baseURL 形如 http://127.0.0.1:8102。
func NewSpecialistGrounder(baseURL string, lg loggateway.Logger) *SpecialistGrounder {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SpecialistGrounder{
		baseURL: strings.TrimRight(baseURL, "/"),
		hc:      &http.Client{Timeout: 30 * time.Second},
		lg:      lg.With(loggateway.Domain("computeruse")),
	}
}

// Pick 本实现走坐标，不选 ref。
func (g *SpecialistGrounder) Pick(ctx context.Context, img bizcu.Image, _ []bizcu.UIElement, target string) (string, error) {
	_, err := g.PickCoordinate(ctx, img, target)
	if err != nil {
		return "", err
	}
	return "", fmt.Errorf("%w: specialist 仅支持坐标输出", bizcu.ErrGroundingFailed)
}

// PickCoordinate 调用远程专用模型。
func (g *SpecialistGrounder) PickCoordinate(ctx context.Context, img bizcu.Image, target string) (bizcu.Point, error) {
	body, _ := json.Marshal(map[string]any{
		"image_base64": base64.StdEncoding.EncodeToString(img.PNG),
		"target":       target,
		"width":        img.Width,
		"height":       img.Height,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/ground", bytes.NewReader(body))
	if err != nil {
		return bizcu.Point{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := g.hc.Do(req)
	if err != nil {
		return bizcu.Point{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		return bizcu.Point{}, fmt.Errorf("computeruse: specialist HTTP %d", resp.StatusCode)
	}
	var out struct {
		X int `json:"x"`
		Y int `json:"y"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return bizcu.Point{}, err
	}
	if out.X < 0 || out.Y < 0 {
		return bizcu.Point{}, bizcu.ErrGroundingFailed
	}
	return bizcu.Point{X: out.X, Y: out.Y}, nil
}
