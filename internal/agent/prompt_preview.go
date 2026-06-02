package agent

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
)

// PreviewSection is one labeled token estimate row for settings UI.
type PreviewSection struct {
	Key       string
	Label     string
	EstTokens int
	Source    string // "build" | "runtime"
}

// PreviewReport is the structured prompt preview for Agent settings.
type PreviewReport struct {
	Instruction          string
	Summary              string
	Sections             []PreviewSection
	StaticTotalTokens    int
	RuntimeOverlayEst    int
	RuntimeNote          string
}

// BuildPreviewReport assembles static instruction text and token breakdown for preview API.
func BuildPreviewReport(ctx context.Context, ag biz.Agent, mode string, deps Deps) PreviewReport {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = strings.ToLower(strings.TrimSpace(ag.SystemPromptMode))
	}
	if mode == "" {
		mode = "complete"
	}
	agPreview := ag
	agPreview.SystemPromptMode = mode

	files := agPreview.Files

	// PGO-1-AGENT-03: resolve category responsibility for preview.
	var catResp string
	if shouldInjectCategoryResponsibility(agPreview) && deps.Taxonomy != nil {
		catResp, _ = deps.Taxonomy.BuildResponsibility(ctx, agPreview.CategoryPositionID, mode)
	}
	sys := BuildSystemPrompt(agPreview, files, mode, catResp)
	instruction := sys
	if cue := RuntimeCapabilityCue(ctx, deps, agPreview); cue != "" {
		if instruction != "" {
			instruction += "\n\n" + cue
		} else {
			instruction = cue
		}
	}

	sections := []PreviewSection{}
	// PGO-1: show role_responsibility (L1) as a distinct section.
	if catResp != "" {
		sections = append(sections, PreviewSection{
			Key: "category_responsibility", Label: "岗位职责 (L1 分类)", EstTokens: estTokensFromChars(utf8.RuneCountInString(catResp)), Source: "build",
		})
	}
	if d := strings.TrimSpace(agPreview.AgentDescription); d != "" {
		sections = append(sections, PreviewSection{
			Key: "description", Label: "Description (L2 个体)", EstTokens: estTokensFromChars(utf8.RuneCountInString(d)), Source: "build",
		})
	}
	fileTokens := 0
	for _, f := range biz.FilesForMode(files, mode) {
		body := strings.TrimSpace(f.Body)
		if body == "" {
			continue
		}
		t := estTokensFromChars(utf8.RuneCountInString(body))
		fileTokens += t
		sections = append(sections, PreviewSection{
			Key: "file:" + f.Name, Label: f.Name, EstTokens: t, Source: "build",
		})
	}
	_ = fileTokens
	if cueTokens := sectionTokens(instruction, "## Runtime capability policy", ""); cueTokens > 0 {
		sections = append(sections, PreviewSection{
			Key: "runtime_cue", Label: "Runtime capability policy", EstTokens: cueTokens, Source: "build",
		})
	}
	if !HasFilteredPromptFile(files, mode, "IDENTITY.md") && strings.TrimSpace(agPreview.DisplayName) != "" {
		idText := "You are " + strings.TrimSpace(agPreview.AgentKey) + ". " + strings.TrimSpace(agPreview.DisplayName)
		sections = append(sections, PreviewSection{
			Key: "identity", Label: "Identity processor", EstTokens: estTokensFromChars(utf8.RuneCountInString(idText)), Source: "runtime",
		})
	}

	runtimeOverlay := 0
	st := agPreview.Settings
	if st != nil {
		mem := biz.ResolveMemoryRuntimePolicy(st)
		if SkillsUseFullProfile(mode) {
			sections = append(sections, PreviewSection{Key: "skills", Label: "Skills 概览 + 目录提示", EstTokens: 700, Source: "runtime"})
			runtimeOverlay += 700
		} else if mode != "none" {
			sections = append(sections, PreviewSection{Key: "skills", Label: "Skills 概览（KnowledgeOnly）", EstTokens: 180, Source: "runtime"})
			runtimeOverlay += 180
		}
		if mem.RecallL2 {
			sections = append(sections, PreviewSection{Key: "l2_memory", Label: "L2 情节记忆", EstTokens: 120, Source: "runtime"})
			runtimeOverlay += 120
		}
		if mem.InjectL3 {
			sections = append(sections, PreviewSection{Key: "l3_memory", Label: "L3 语义记忆", EstTokens: 150, Source: "runtime"})
			runtimeOverlay += 150
		}
		if mem.InjectL4 {
			sections = append(sections, PreviewSection{Key: "l4_memory", Label: "L4 知识图谱", EstTokens: 200, Source: "runtime"})
			runtimeOverlay += 200
		}
		if mem.InjectL1 {
			sections = append(sections, PreviewSection{Key: "l1_memory", Label: "L1 工作记忆", EstTokens: 100, Source: "runtime"})
			runtimeOverlay += 100
		}
		if st.IntentPassEnabled {
			sections = append(sections, PreviewSection{Key: "intent", Label: "Intent Pass 上下文", EstTokens: 100, Source: "runtime"})
			runtimeOverlay += 100
		}
		if st.SessionSummaryEnabled {
			sections = append(sections, PreviewSection{Key: "session_summary", Label: "Session 摘要", EstTokens: 350, Source: "runtime"})
			runtimeOverlay += 350
		}
		if mem.MasterEnabled {
			sections = append(sections, PreviewSection{Key: "user_memories", Label: "Framework Memory Preload", EstTokens: 200, Source: "runtime"})
			runtimeOverlay += 200
		}
	}
	sections = append(sections, PreviewSection{
		Key: "history", Label: "对话历史 + 工具结果", EstTokens: 0, Source: "runtime",
	})

	staticTotal := estTokensFromChars(utf8.RuneCountInString(instruction))
	runtimeNote := "静态 instruction 为构建期固定内容。L2/L3/L4、Skills、Intent、Session 摘要与历史在每轮 LLM 调用时动态追加；实际 token 以 Monitor 中 chat.prompt.compose 为准。"

	return PreviewReport{
		Instruction:       instruction,
		Summary:           composePromptSummary(agPreview, mode),
		Sections:          sections,
		StaticTotalTokens: staticTotal,
		RuntimeOverlayEst: runtimeOverlay,
		RuntimeNote:       runtimeNote,
	}
}

func composePromptSummary(ag biz.Agent, mode string) string {
	var b strings.Builder
	b.WriteString("# Prompt 预览说明\n\n")
	fmt.Fprintf(&b, "模式: %s | Agent: %s (%s)\n\n", mode, ag.DisplayName, ag.AgentKey)
	b.WriteString("下方 **Instruction** 为构建期写入模型的静态 system 内容（Description + Prompt 文件 + Runtime 策略）。\n")
	b.WriteString("运行时还会注入 Identity、Skills、记忆、Intent、Session 摘要与对话历史；见 sections 分解与 runtime_note。\n")
	if ag.Settings != nil {
		b.WriteString("\n## 关键开关\n")
		fmt.Fprintf(&b, "- Intent Pass: %t\n", ag.Settings.IntentPassEnabled)
		fmt.Fprintf(&b, "- L2 / L3 / L4 注入: %t / %t / %t\n",
			ag.Settings.L2RecallEnabled,
			ag.Settings.L3Enabled && ag.Settings.L0InjectL3,
			ag.Settings.L4Enabled && ag.Settings.L0InjectL4,
		)
		fmt.Fprintf(&b, "- Session Summary: %t\n", ag.Settings.SessionSummaryEnabled)
	}
	return strings.TrimSpace(b.String())
}
