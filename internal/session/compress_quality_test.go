package session

import (
	"errors"
	"fmt"
	"testing"

	"aranea-agents/pkg/apierror"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestIsDegenerateSummary(t *testing.T) {
	longTranscript := 5000 // runes
	short := ""
	for i := 0; i < 100; i++ {
		short += "短"
	}
	if !isDegenerateSummary(short, longTranscript) {
		t.Fatal("100 字摘要 vs 5000 字原文应判退化")
	}
	long := short + short + short // 300 字
	if isDegenerateSummary(long, longTranscript) {
		t.Fatal("300 字摘要不应判退化")
	}
	if isDegenerateSummary(short, 500) {
		t.Fatal("原文不足 1000 字时不启用退化判定")
	}
}

func TestPassesReductionGuard(t *testing.T) {
	if !passesReductionGuard(700, 1000) {
		t.Fatal("降到 70% 应通过")
	}
	if passesReductionGuard(850, 1000) {
		t.Fatal("只降到 85% 应被守卫拦截")
	}
	if !passesReductionGuard(0, 0) {
		t.Fatal("零输入不应拦截（无意义比较放过）")
	}
}

func TestClassifyCompressError(t *testing.T) {
	strPtr := func(s string) *string { return &s }
	cases := []struct {
		name string
		err  error
		want compressFailureKind
	}{
		{"nil", nil, compressFailureNone},
		{"context_overflow_message", apierror.Wrap(&trpcmodel.ResponseError{Message: "This model's maximum context length is 131072 tokens"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"context_code", apierror.Wrap(&trpcmodel.ResponseError{Message: "bad request", Code: strPtr("context_length_exceeded")}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"invalid_request_type", apierror.Wrap(&trpcmodel.ResponseError{Message: "bad param", Type: "invalid_request_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"auth_type", apierror.Wrap(&trpcmodel.ResponseError{Message: "invalid api key", Type: "authentication_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureDeterministic},
		{"rate_limit", apierror.Wrap(&trpcmodel.ResponseError{Message: "slow down", Type: "rate_limit_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureTransient},
		{"server_5xx", apierror.Wrap(&trpcmodel.ResponseError{Message: "internal error", Type: "server_error"}, apierror.CodeInternal, apierror.DomainProvider), compressFailureTransient},
		{"plain_unknown", fmt.Errorf("network blip"), compressFailureTransient},
		{"wrapped_unknown", fmt.Errorf("wrap: %w", errors.New("io timeout")), compressFailureTransient},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyCompressError(tt.err); got != tt.want {
				t.Fatalf("classifyCompressError = %v, want %v", got, tt.want)
			}
		})
	}
}
