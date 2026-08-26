package validate

import (
	"strings"
	"testing"

	chatv1 "aranea-agents/api/kratos/chat/v1"
)

func boolPtr(b bool) *bool { return &b }

// 验收3 根修回归：proto3 optional bool 显式 false（HITL 拒绝）必须通过
// REQUIRED presence 校验；真正缺失（nil）必须拒绝。
func TestValidateRequiredFields_OptionalBoolPresence(t *testing.T) {
	cases := []struct {
		name    string
		req     *chatv1.ConfirmActivityRequest
		wantErr string // 空 = 期望通过；非空 = 期望错误消息包含该串
	}{
		{
			name: "explicit false passes (reject path)",
			req: &chatv1.ConfirmActivityRequest{
				SessionId:  "s1",
				ActivityId: "a1",
				Approved:   boolPtr(false),
			},
		},
		{
			name: "explicit true passes (approve path)",
			req: &chatv1.ConfirmActivityRequest{
				SessionId:  "s1",
				ActivityId: "a1",
				Approved:   boolPtr(true),
			},
		},
		{
			name: "nil approved rejected",
			req: &chatv1.ConfirmActivityRequest{
				SessionId:  "s1",
				ActivityId: "a1",
			},
			wantErr: "missing required field: approved",
		},
		{
			name: "empty session_id rejected (implicit presence keeps value semantics)",
			req: &chatv1.ConfirmActivityRequest{
				ActivityId: "a1",
				Approved:   boolPtr(true),
			},
			wantErr: "missing required field: session_id",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateRequiredFields(tc.req)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// ConfirmPlanRequest 同款 optional approved 字段必须同语义。
func TestValidateRequiredFields_ConfirmPlanOptionalBool(t *testing.T) {
	err := ValidateRequiredFields(&chatv1.ConfirmPlanRequest{
		PlanId:    "p1",
		SessionId: "s1",
		Approved:  boolPtr(false),
	})
	if err != nil {
		t.Fatalf("explicit false must pass: %v", err)
	}
}
