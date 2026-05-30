package artifact_test

import (
	"testing"

	"aranea-agents/internal/biz/artifact"
)

func TestNormalizeAttachmentIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		want []string
	}{
		{
			name: "nil_input",
			ids:  nil,
			want: []string{},
		},
		{
			name: "empty_input",
			ids:  []string{},
			want: []string{},
		},
		{
			name: "all_valid",
			ids:  []string{"a1", "b2", "c3"},
			want: []string{"a1", "b2", "c3"},
		},
		{
			name: "trims_whitespace",
			ids:  []string{"  a1  ", " b2", "c3 "},
			want: []string{"a1", "b2", "c3"},
		},
		{
			name: "drops_empty_strings",
			ids:  []string{"a1", "", "c3"},
			want: []string{"a1", "c3"},
		},
		{
			name: "drops_whitespace_only",
			ids:  []string{"a1", "   ", "\t", "\n", "c3"},
			want: []string{"a1", "c3"},
		},
		{
			name: "all_empty_and_whitespace",
			ids:  []string{"", "  ", "\t", "\n"},
			want: []string{},
		},
		{
			name: "mixed_valid_empty_whitespace",
			ids:  []string{"  id-1  ", "", "  ", "id-2", "\t", "  id-3  "},
			want: []string{"id-1", "id-2", "id-3"},
		},
		{
			name: "preserves_order",
			ids:  []string{"c3", "  a1  ", "b2"},
			want: []string{"c3", "a1", "b2"},
		},
		{
			name: "single_valid_id",
			ids:  []string{"only-one"},
			want: []string{"only-one"},
		},
		{
			name: "single_whitespace_only",
			ids:  []string{"   "},
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artifact.NormalizeAttachmentIDs(tt.ids)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("item[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
