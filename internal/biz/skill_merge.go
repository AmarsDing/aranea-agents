package biz

import (
	"context"
	"fmt"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"

	"aranea-agents/pkg/loggateway"
)

// MergeStrategy 合并策略
type MergeStrategy string

const (
	MergeStrategyAIFuse     MergeStrategy = "ai_fuse"     // AI 自动融合
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
		return nil, kerrors.BadRequest("SKILL_MERGE", "source and target must be different skills")
	}

	// 获取完整数据
	source, err := uc.reader.GetFullSkillForMerge(ctx, req.SourceID)
	if err != nil {
		return nil, fmt.Errorf("get source skill: %w", err)
	}
	target, err := uc.reader.GetFullSkillForMerge(ctx, req.TargetID)
	if err != nil {
		return nil, fmt.Errorf("get target skill: %w", err)
	}

	// Stage 1: 内容融合
	var fused *FusedContent
	switch req.Strategy {
	case MergeStrategyAIFuse:
		if uc.fuser == nil {
			return nil, kerrors.BadRequest("SKILL_MERGE", "AI fusion not available, use manual_pick or append strategy")
		}
		fused, err = uc.fuser.Fuse(ctx, *target, *source)
		if err != nil {
			return nil, fmt.Errorf("AI fusion failed: %w", err)
		}
	case MergeStrategyManualPick:
		fused = &FusedContent{
			Body: req.ManualBody,
			Tags: mergeStringSets(target.Tags, source.Tags),
		}
	case MergeStrategyAppend:
		fused = &FusedContent{
			Body: target.Body + "\n\n---\n\n# Merged from: " + source.Name + "\n\n" + source.Body,
			Tags: mergeStringSets(target.Tags, source.Tags),
		}
	default:
		return nil, kerrors.BadRequest("SKILL_MERGE", fmt.Sprintf("unknown merge strategy: %s", req.Strategy))
	}

	// Stage 2: Gate 验证（安全检查 + 结构检查）
	if uc.gate != nil {
		gateResult, gateErr := uc.gate.Verify(ctx, req.TargetID, fused.Body, nil)
		if gateErr != nil {
			return nil, fmt.Errorf("gate verification error: %w", gateErr)
		}
		if gateResult != nil && !gateResult.Passed {
			return nil, kerrors.BadRequest("SKILL_MERGE", fmt.Sprintf("gate verification failed: %s", formatGateFailures(gateResult)))
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
		return nil, fmt.Errorf("apply merge: %w", err)
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
