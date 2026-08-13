package computeruse

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// vlmPickTimeout 单次 VLM 定位调用超时（容忍本地模型冷启动加载 + 大图 prefill）。
const vlmPickTimeout = 60 * time.Second

// vlmImageMaxSide 发送给 VLM 的截图最长边上限：降采样减少视觉 token 与 prefill
// 耗时（本地 7B 模型上全尺寸截图 prefill 可达数十秒）。grounding 语义不受影响：
// SoM bbox 文本同比例缩放、坐标直判走归一化千分位。
const vlmImageMaxSide = 1568

// vlmNumberPattern 提取 VLM 回复中的首个整数编号（容忍啰嗦前后缀；
// 允许负号以识别 "-1" 无匹配哨兵——禁止只取数字部分误选候选 1）。
var vlmNumberPattern = regexp.MustCompile(`-?\d+`)

// vlmCoordPattern 提取 VLM 回复中的首个归一化坐标对（0-1000 千分位；容忍全角逗号/括号；
// 允许负号以识别 "-1, -1" 无匹配哨兵）。
var vlmCoordPattern = regexp.MustCompile(`(-?\d{1,4})\s*[,，]\s*(-?\d{1,4})`)

// VisionLLMSettingsGetter 系统设置来源（与 knowledge.RefineLLMSettingsGetter 同签名，
// service 层装配时由 *biz.SystemSettingUsecase 满足）。
type VisionLLMSettingsGetter interface {
	Get(ctx context.Context) (biz.SystemSetting, error)
}

// VisionLLMCatalogLister 模型目录来源（同 knowledge.LLMCatalogLister 签名）。
type VisionLLMCatalogLister interface {
	List(ctx context.Context) ([]biz.ProviderModel, error)
}

// VLMGrounder 实现 bizcu.VisionGrounder：SoM 标注图 + 候选列表 → VLM 选编号。
type VLMGrounder struct {
	llm     biz.LLMCaller
	sys     VisionLLMSettingsGetter // 可 nil
	catalog VisionLLMCatalogLister  // 可 nil
	lg      loggateway.Logger
}

// NewVLMGrounder 构造 VLM 定位器；llm 必填，sys/catalog 可 nil（解析时互为降级）。
func NewVLMGrounder(llm biz.LLMCaller, sys VisionLLMSettingsGetter, catalog VisionLLMCatalogLister, lg loggateway.Logger) *VLMGrounder {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VLMGrounder{llm: llm, sys: sys, catalog: catalog, lg: lg}
}

var _ bizcu.VisionGrounder = (*VLMGrounder)(nil)

// Pick 从候选元素中为 target 选出最匹配者，返回其 ref。
func (g *VLMGrounder) Pick(ctx context.Context, img bizcu.Image, candidates []bizcu.UIElement, target string) (string, error) {
	if len(candidates) == 0 {
		return "", fmt.Errorf("%w: 视觉兜底无候选元素", bizcu.ErrGroundingFailed)
	}
	provider, model, err := g.resolveVisionLLM(ctx)
	if err != nil {
		return "", err
	}
	annotated, err := bizcu.AnnotateSoM(img, candidates)
	if err != nil {
		return "", err
	}
	// 降采样发送图；prompt 中 bbox 文本同比例缩放，保持与图像素一致。
	scale := 1.0
	if s, f, derr := bizcu.DownscalePNG(annotated, vlmImageMaxSide); derr == nil {
		annotated, scale = s, f
	}

	callCtx, cancel := context.WithTimeout(ctx, vlmPickTimeout)
	defer cancel()
	resp, _, err := g.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是一名桌面 UI 元素定位助手。截图上已用红色编号框标注候选元素。
根据用户目标选择最匹配的一个元素。只输出该元素的编号数字，不要任何解释。
若所有候选都与目标无匹配，输出 0。`,
		User:   buildVLMPrompt(candidates, target, scale),
		Images: []biz.LLMImage{{Data: annotated, Format: "png"}},
	})
	if err != nil {
		return "", fmt.Errorf("%w: VLM 调用失败: %v", bizcu.ErrGroundingFailed, err)
	}

	n, err := parseVLMNumber(resp, len(candidates))
	if err != nil {
		return "", err
	}
	return candidates[n-1].Ref, nil
}

// PickCoordinate 实现 vlm_direct 路径：VLM 直出目标在截图上的归一化千分位坐标，换算为图像素点。
func (g *VLMGrounder) PickCoordinate(ctx context.Context, img bizcu.Image, target string) (bizcu.Point, error) {
	if len(img.PNG) == 0 || img.Width <= 0 || img.Height <= 0 {
		return bizcu.Point{}, fmt.Errorf("%w: 坐标直判无有效截图", bizcu.ErrGroundingFailed)
	}
	provider, model, err := g.resolveVisionLLM(ctx)
	if err != nil {
		return bizcu.Point{}, err
	}
	// 降采样发送图；归一化坐标与分辨率无关，换算仍按原始宽高。
	send := img.PNG
	if s, _, derr := bizcu.DownscalePNG(img.PNG, vlmImageMaxSide); derr == nil {
		send = s
	}
	callCtx, cancel := context.WithTimeout(ctx, vlmPickTimeout)
	defer cancel()
	resp, _, err := g.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是一名桌面 UI 元素定位助手。根据用户目标，在截图上找到最匹配的元素。
输出该中心点的归一化坐标：x 与 y 均为 0-1000 的整数（千分位，左上角为 0,0）。
只输出 "x, y"，不要任何解释。若截图中没有与目标匹配的元素，输出 "-1, -1"。`,
		User:   fmt.Sprintf("目标：%s\n只输出归一化坐标 \"x, y\"；无匹配输出 \"-1, -1\"。", target),
		Images: []biz.LLMImage{{Data: send, Format: "png"}},
	})
	if err != nil {
		return bizcu.Point{}, fmt.Errorf("%w: VLM 调用失败: %v", bizcu.ErrGroundingFailed, err)
	}
	return parseNormalizedPoint(resp, img.Width, img.Height)
}

// parseNormalizedPoint 解析归一化千分位坐标并换算为图像素点；
// 负值为无匹配哨兵（-1, -1），越界（>1000）报错。
func parseNormalizedPoint(resp string, w, h int) (bizcu.Point, error) {
	m := vlmCoordPattern.FindStringSubmatch(resp)
	if m == nil {
		return bizcu.Point{}, fmt.Errorf("%w: VLM 回复无坐标: %q", bizcu.ErrGroundingFailed, resp)
	}
	nx, _ := strconv.Atoi(m[1])
	ny, _ := strconv.Atoi(m[2])
	if nx < 0 || ny < 0 {
		return bizcu.Point{}, fmt.Errorf("%w: VLM 判定无匹配元素", bizcu.ErrGroundingFailed)
	}
	if nx > 1000 || ny > 1000 {
		return bizcu.Point{}, fmt.Errorf("%w: VLM 坐标越界: %q（应 0-1000）", bizcu.ErrGroundingFailed, m[0])
	}
	return bizcu.Point{X: nx * w / 1000, Y: ny * h / 1000}, nil
}

// buildVLMPrompt 构造候选列表 + 目标的用户提示（编号从 1 开始，与 SoM 标注一致）。
// scale 为发送图降采样因子：bbox 文本同比例缩放，与 VLM 实际所见图像素一致。
func buildVLMPrompt(candidates []bizcu.UIElement, target string, scale float64) string {
	if scale <= 0 {
		scale = 1
	}
	var sb strings.Builder
	sb.WriteString("候选元素：\n")
	for i, el := range candidates {
		fmt.Fprintf(&sb, "%d. [%s] %q 位置(%d,%d %dx%d)\n",
			i+1, el.Type, el.Name,
			int(float64(el.BBox.X)*scale), int(float64(el.BBox.Y)*scale),
			int(float64(el.BBox.W)*scale), int(float64(el.BBox.H)*scale))
	}
	fmt.Fprintf(&sb, "目标：%s\n只输出编号数字；无匹配输出 0。", target)
	return sb.String()
}

// parseVLMNumber 从 VLM 回复提取首个整数并校验范围：0 = VLM 判定无匹配（明确失败），
// [1, n] = 候选编号，其余越界报错。
func parseVLMNumber(resp string, n int) (int, error) {
	m := vlmNumberPattern.FindString(resp)
	if m == "" {
		return 0, fmt.Errorf("%w: VLM 回复无编号: %q", bizcu.ErrGroundingFailed, resp)
	}
	var v int
	if _, err := fmt.Sscanf(m, "%d", &v); err != nil {
		return 0, fmt.Errorf("%w: VLM 编号解析失败: %q", bizcu.ErrGroundingFailed, m)
	}
	if v == 0 {
		return 0, fmt.Errorf("%w: VLM 判定无匹配元素", bizcu.ErrGroundingFailed)
	}
	if v < 0 {
		return 0, fmt.Errorf("%w: VLM 负号哨兵（无匹配）: %q", bizcu.ErrGroundingFailed, m)
	}
	if v < 1 || v > n {
		return 0, fmt.Errorf("%w: VLM 编号越界: %q（候选 %d 个）", bizcu.ErrGroundingFailed, m, n)
	}
	return v, nil
}

// resolveVisionLLM 解析具备视觉能力的 LLM（模式同 knowledge.ResolveVisionLLM）：
// 优先目录中显式声明 Vision 能力的启用模型；无则回退 DefaultRefineLLM；两者皆无报错。
func (g *VLMGrounder) resolveVisionLLM(ctx context.Context) (string, string, error) {
	if g.catalog != nil {
		models, err := g.catalog.List(ctx)
		if err != nil {
			g.lg.Warn("模型目录获取失败",
				loggateway.StepID("computeruse.resolve_vision_llm"),
				loggateway.Err(err))
		} else {
			for _, m := range models {
				if m.Provider != "" && m.Model != "" && m.Enabled && m.Capabilities.Vision {
					return m.Provider, m.Model, nil
				}
			}
		}
	}
	if g.sys != nil {
		s, err := g.sys.Get(ctx)
		if err != nil {
			g.lg.Warn("系统设置获取失败",
				loggateway.StepID("computeruse.resolve_vision_llm"),
				loggateway.Err(err))
		} else if strings.TrimSpace(s.DefaultRefineLLM.Provider) != "" && strings.TrimSpace(s.DefaultRefineLLM.Model) != "" {
			return s.DefaultRefineLLM.Provider, s.DefaultRefineLLM.Model, nil
		}
	}
	return "", "", apierror.Unavailable(apierror.DomainComputerUse,
		"no vision-capable LLM available for computer-use grounding; enable a vision model in the catalog or configure DefaultRefineLLM")
}
