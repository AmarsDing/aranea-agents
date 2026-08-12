package computeruse

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// vlmPickTimeout 单次 VLM 定位调用超时。
const vlmPickTimeout = 30 * time.Second

// vlmNumberPattern 提取 VLM 回复中的首个整数编号（容忍啰嗦前后缀）。
var vlmNumberPattern = regexp.MustCompile(`\d+`)

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

	callCtx, cancel := context.WithTimeout(ctx, vlmPickTimeout)
	defer cancel()
	resp, _, err := g.llm.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System: `你是一名桌面 UI 元素定位助手。截图上已用红色编号框标注候选元素。
根据用户目标选择最匹配的一个元素。只输出该元素的编号数字，不要任何解释。`,
		User:   buildVLMPrompt(candidates, target),
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

// buildVLMPrompt 构造候选列表 + 目标的用户提示（编号从 1 开始，与 SoM 标注一致）。
func buildVLMPrompt(candidates []bizcu.UIElement, target string) string {
	var sb strings.Builder
	sb.WriteString("候选元素：\n")
	for i, el := range candidates {
		fmt.Fprintf(&sb, "%d. [%s] %q 位置(%d,%d %dx%d)\n",
			i+1, el.Type, el.Name, el.BBox.X, el.BBox.Y, el.BBox.W, el.BBox.H)
	}
	fmt.Fprintf(&sb, "目标：%s\n只输出编号数字。", target)
	return sb.String()
}

// parseVLMNumber 从 VLM 回复提取首个整数并校验范围 [1, n]。
func parseVLMNumber(resp string, n int) (int, error) {
	m := vlmNumberPattern.FindString(resp)
	if m == "" {
		return 0, fmt.Errorf("%w: VLM 回复无编号: %q", bizcu.ErrGroundingFailed, resp)
	}
	var v int
	if _, err := fmt.Sscanf(m, "%d", &v); err != nil || v < 1 || v > n {
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
