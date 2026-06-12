package biz

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// MergeStrategy 合并策略
type MergeStrategy string

const (
	MergeStrategyAIFuse     MergeStrategy = "ai_fuse"     // Deprecated: Use MergeStrategyRuleFuse
	MergeStrategyRuleFuse   MergeStrategy = "rule_fuse"   // Rule-based automatic fusion
	MergeStrategyManualPick MergeStrategy = "manual_pick" // 人工选择保留哪个
	MergeStrategyAppend     MergeStrategy = "append"      // 追加源内容到目标末尾
)

// SkillMergeRequest 合并请求
type SkillMergeRequest struct {
	SourceID   string
	TargetID   string
	Strategy   MergeStrategy
	ManualBody string // manual_pick 时由用户提供
}

// SkillMergeResult 合并结果
type SkillMergeResult struct {
	TargetSkillID    string
	NewVersionID     string
	FusedBody        string
	FusedTags        []string
	TransferredCount int
}

// SkillMergeSource 合并源数据
type SkillMergeSource struct {
	ID          string
	Name        string
	Description string
	Body        string // 完整 SKILL.md
	Tags        []string
}

// FusedContent 融合结果
type FusedContent struct {
	Body string
	Tags []string
}

// SkillContentFuser 内容融合器接口
type SkillContentFuser interface {
	Fuse(ctx context.Context, target SkillMergeSource, source SkillMergeSource) (*FusedContent, error)
}

// SkillMergeReader 读取合并所需的完整数据
type SkillMergeReader interface {
	GetFullSkillForMerge(ctx context.Context, skillID string) (*SkillMergeSource, error)
}

// SkillMergeWriter 事务性写入合并结果
type SkillMergeWriter interface {
	ApplyMerge(ctx context.Context, params SkillMergeApplyParams) (*SkillMergeResult, error)
}

// SkillMergeApplyParams 事务应用参数
type SkillMergeApplyParams struct {
	TargetID    string
	SourceID    string
	FusedBody   string
	FusedTags   []string
	MergeReason string
}

// SkillMergeUsecase 增强的合并 Usecase
type SkillMergeUsecase struct {
	reader SkillMergeReader
	writer SkillMergeWriter
	fuser  SkillContentFuser
	gate   SkillGateVerifier
	lg     loggateway.Logger
}

func NewSkillMergeUsecase(
	reader SkillMergeReader,
	writer SkillMergeWriter,
	fuser SkillContentFuser,
	gate SkillGateVerifier,
	lg loggateway.Logger,
) *SkillMergeUsecase {
	return &SkillMergeUsecase{
		reader: reader,
		writer: writer,
		fuser:  fuser,
		gate:   gate,
		lg:     lg,
	}
}

// Merge 执行三阶段合并
func (uc *SkillMergeUsecase) Merge(ctx context.Context, req SkillMergeRequest) (*SkillMergeResult, error) {
	if req.SourceID == req.TargetID {
		return nil, apierror.BadRequest("SKILL_MERGE", "source and target must be different skills")
	}

	// 获取完整数据
	source, err := uc.reader.GetFullSkillForMerge(ctx, req.SourceID)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_MERGE")
	}
	target, err := uc.reader.GetFullSkillForMerge(ctx, req.TargetID)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_MERGE")
	}

	// Stage 1: 内容融合
	var fused *FusedContent
	switch req.Strategy {
	case MergeStrategyAIFuse, MergeStrategyRuleFuse:
		if uc.fuser == nil {
			return nil, apierror.BadRequest("SKILL_MERGE", "fusion not available, use manual_pick or append strategy")
		}
		fused, err = uc.fuser.Fuse(ctx, *target, *source)
		if err != nil {
			return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_MERGE")
		}
	case MergeStrategyManualPick:
		if strings.TrimSpace(req.ManualBody) == "" {
			return nil, apierror.BadRequest("SKILL_MERGE", "manual_body is required for manual_pick strategy")
		}
		fused = &FusedContent{
			Body: req.ManualBody,
			Tags: mergeStringSets(target.Tags, source.Tags),
		}
	case MergeStrategyAppend:
		fused = &FusedContent{
			Body: appendWithDedup(target.Body, source.Body, source.Name, uc.lg),
			Tags: mergeStringSets(target.Tags, source.Tags),
		}
	default:
		return nil, apierror.BadRequest("SKILL_MERGE", "unknown merge strategy: %s", req.Strategy)
	}

	// Stage 2: Gate 验证（安全检查 + 结构检查）
	if uc.gate != nil {
		gateResult, gateErr := uc.gate.Verify(ctx, req.TargetID, fused.Body, nil)
		if gateErr != nil {
			return nil, apierror.Wrap(gateErr, apierror.CodeInternal, "SKILL_MERGE")
		}
		if gateResult != nil && !gateResult.Passed {
			return nil, apierror.BadRequest("SKILL_MERGE", "gate verification failed: %s", formatGateFailures(gateResult))
		}
	}

	// Stage 3: 事务应用
	mergeReason := fmt.Sprintf("merged from skill %s (%s)", source.ID, source.Name)
	result, err := uc.writer.ApplyMerge(ctx, SkillMergeApplyParams{
		TargetID:    req.TargetID,
		SourceID:    req.SourceID,
		FusedBody:   fused.Body,
		FusedTags:   fused.Tags,
		MergeReason: mergeReason,
	})
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_MERGE")
	}

	uc.lg.Info("skill merged with content fusion",
		loggateway.StepID("skill_merge.merge"),
		loggateway.Str("source_id", req.SourceID),
		loggateway.Str("target_id", req.TargetID),
		loggateway.Str("strategy", string(req.Strategy)))

	return result, nil
}

func mergeStringSets(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	var result []string
	for _, s := range a {
		lower := strings.ToLower(s)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, s)
		}
	}
	for _, s := range b {
		lower := strings.ToLower(s)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, s)
		}
	}
	return result
}

func formatGateFailures(result *GateVerificationResult) string {
	var failures []string
	for _, c := range result.Checks {
		if !c.Passed {
			failures = append(failures, c.Name+": "+c.Reason)
		}
	}
	return strings.Join(failures, "; ")
}

// appendWithDedup appends source body to target body, skipping sections that
// already exist in the target (based on ## heading deduplication).
func appendWithDedup(targetBody, sourceBody, sourceName string, lg loggateway.Logger) string {
	targetHeadings := extractSectionHeadings(targetBody)
	sourceSections := extractSections(sourceBody)

	var b strings.Builder
	b.WriteString(targetBody)
	b.WriteString("\n\n---\n\n# Merged from: ")
	b.WriteString(sourceName)
	b.WriteString("\n")

	skippedCount := 0
	for heading, content := range sourceSections {
		if targetHeadings[heading] {
			skippedCount++
			lg.Debug("skill_merge: skipping duplicate section in append",
				loggateway.StepID("skill_merge.append_dedup"),
				loggateway.Str("heading", heading))
			continue
		}
		b.WriteString("\n")
		b.WriteString(content)
	}

	if skippedCount > 0 {
		lg.Info("skill_merge: append dedup skipped duplicate sections",
			loggateway.StepID("skill_merge.append_dedup"),
			loggateway.Int("skipped_count", skippedCount))
	}

	return b.String()
}
