package service

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// 硬上限（biz.UserInputHardLimitChars）在闸前拒绝：API/WS 绕过前端校验时
// 不静默转存 blob，与 attachments.go 团队路径行为对齐（2026-07-27 review 修复）。
// 硬上限检查位于 p.rt() 之前，零值 turnPipeline 即可测试。
func TestGateTurnUserInput_HardLimitRejected(t *testing.T) {
	p := &turnPipeline{}
	huge := strings.Repeat("超", biz.UserInputHardLimitChars+1)
	_, err := p.gateTurnUserInput(context.Background(), "sess-1", huge)
	if err == nil {
		t.Fatal("expected hard-limit error for input exceeding UserInputHardLimitChars")
	}
	apiErr, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected *apierror.Error, got %T: %v", err, err)
	}
	if apiErr.Code != apierror.CodeBadRequest {
		t.Fatalf("expected CodeBadRequest, got %s", apiErr.Code)
	}
}