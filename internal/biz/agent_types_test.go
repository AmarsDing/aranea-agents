package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestSkipCategoryResponsibility(t *testing.T) {
	tests := []struct {
		name         string
		metadataJSON string
		want         bool
	}{
		{"empty string", "", false},
		{"whitespace only", "   ", false},
		{"skip true", `{"skip_category_responsibility":true}`, true},
		{"skip false", `{"skip_category_responsibility":false}`, false},
		{"other field only", `{"other_field":123}`, false},
		{"invalid json", `{not json}`, false},
		{"skip true with other fields", `{"skip_category_responsibility":true,"other":"val"}`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := biz.Agent{MetadataJSON: tt.metadataJSON}
			if got := a.SkipCategoryResponsibility(); got != tt.want {
				t.Errorf("SkipCategoryResponsibility() = %v, want %v", got, tt.want)
			}
		})
	}
}
