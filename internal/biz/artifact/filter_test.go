package artifact_test

import (
	"testing"

	"aranea-agents/internal/biz/artifact"
)

func TestFilterArtifacts(t *testing.T) {
	items := []artifact.Artifact{
		{ID: "1", Name: "report.csv", MimeType: "text/csv", SessionID: "sess-a"},
		{ID: "2", Name: "photo.png", MimeType: "image/png", SessionID: "sess-a"},
		{ID: "3", Name: "data.json", MimeType: "application/json", SessionID: "sess-b"},
		{ID: "4", Name: "chart.svg", MimeType: "image/svg+xml", SessionID: "sess-b"},
		{ID: "5", Name: "notes.txt", MimeType: "text/plain", SessionID: "sess-c"},
	}

	tests := []struct {
		name       string
		items      []artifact.Artifact
		query      string
		mimePrefix string
		wantIDs    []string
	}{
		{
			name:       "empty_query_and_prefix_returns_all",
			items:      items,
			query:      "",
			mimePrefix: "",
			wantIDs:    []string{"1", "2", "3", "4", "5"},
		},
		{
			name:       "whitespace_query_and_prefix_returns_all",
			items:      items,
			query:      "   ",
			mimePrefix: "  ",
			wantIDs:    []string{"1", "2", "3", "4", "5"},
		},
		{
			name:       "name_match",
			items:      items,
			query:      "report",
			mimePrefix: "",
			wantIDs:    []string{"1"},
		},
		{
			name:       "name_match_case_insensitive",
			items:      items,
			query:      "REPORT",
			mimePrefix: "",
			wantIDs:    []string{"1"},
		},
		{
			name:       "mime_match",
			items:      items,
			query:      "image/png",
			mimePrefix: "",
			wantIDs:    []string{"2"},
		},
		{
			name:       "session_id_match",
			items:      items,
			query:      "sess-b",
			mimePrefix: "",
			wantIDs:    []string{"3", "4"},
		},
		{
			name:       "mime_prefix_image",
			items:      items,
			query:      "",
			mimePrefix: "image/",
			wantIDs:    []string{"2", "4"},
		},
		{
			name:       "mime_prefix_text",
			items:      items,
			query:      "",
			mimePrefix: "text/",
			wantIDs:    []string{"1", "5"},
		},
		{
			name:       "mime_prefix_case_insensitive",
			items:      items,
			query:      "",
			mimePrefix: "Image/",
			wantIDs:    []string{"2", "4"},
		},
		{
			name:       "no_matches",
			items:      items,
			query:      "nonexistent",
			mimePrefix: "",
			wantIDs:    nil,
		},
		{
			name:       "no_matches_mime_prefix",
			items:      items,
			query:      "",
			mimePrefix: "video/",
			wantIDs:    nil,
		},
		{
			name:       "combined_query_and_mime_prefix",
			items:      items,
			query:      "sess-a",
			mimePrefix: "image/",
			wantIDs:    []string{"2"},
		},
		{
			name:       "combined_query_and_mime_prefix_no_overlap",
			items:      items,
			query:      "notes",
			mimePrefix: "image/",
			wantIDs:    nil,
		},
		{
			name:       "empty_items",
			items:      nil,
			query:      "report",
			mimePrefix: "",
			wantIDs:    nil,
		},
		{
			name:       "partial_name_match",
			items:      items,
			query:      ".csv",
			mimePrefix: "",
			wantIDs:    []string{"1"},
		},
		{
			name:       "mime_prefix_application",
			items:      items,
			query:      "",
			mimePrefix: "application/",
			wantIDs:    []string{"3"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := artifact.FilterArtifacts(tt.items, tt.query, tt.mimePrefix)
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("got %d items, want %d", len(got), len(tt.wantIDs))
			}
			for i, g := range got {
				if g.ID != tt.wantIDs[i] {
					t.Errorf("item[%d]: got ID=%q, want %q", i, g.ID, tt.wantIDs[i])
				}
			}
		})
	}
}
