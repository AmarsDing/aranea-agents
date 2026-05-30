package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestGraphValidationResult_HasErrors(t *testing.T) {
	cases := []struct {
		name string
		r    *biz.GraphValidationResult
		want bool
	}{
		{"nil result", nil, false},
		{"empty errors", &biz.GraphValidationResult{}, false},
		{"with errors", &biz.GraphValidationResult{
			Errors: []biz.GraphValidationIssue{{Code: "E1", Message: "err"}},
		}, true},
		{"only warnings no errors", &biz.GraphValidationResult{
			Warnings: []biz.GraphValidationIssue{{Code: "W1", Message: "warn"}},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.r.HasErrors()
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestGraphValidationResult_HasWarnings(t *testing.T) {
	cases := []struct {
		name string
		r    *biz.GraphValidationResult
		want bool
	}{
		{"nil result", nil, false},
		{"empty warnings", &biz.GraphValidationResult{}, false},
		{"with warnings", &biz.GraphValidationResult{
			Warnings: []biz.GraphValidationIssue{{Code: "W1", Message: "warn"}},
		}, true},
		{"only errors no warnings", &biz.GraphValidationResult{
			Errors: []biz.GraphValidationIssue{{Code: "E1", Message: "err"}},
		}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.r.HasWarnings()
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}
