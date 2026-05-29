package session

import (
	"strings"
	"testing"
)

func TestSanitizeExportFilename(t *testing.T) {
	tests := []struct {
		name  string
		title string
		id    string
		want  string
	}{
		{
			name:  "normal name",
			title: "My Chat Session",
			id:    "abc123",
			want:  "My-Chat-Session",
		},
		{
			name:  "special chars stripped",
			title: `Hello/World:Test?File*Name<>"|\\end`,
			id:    "abc123",
			want:  "HelloWorldTestFileNameend",
		},
		{
			name:  "empty title falls back to session id",
			title: "",
			id:    "abc123",
			want:  "session-abc123",
		},
		{
			name:  "whitespace only title falls back",
			title: "   ",
			id:    "xyz789",
			want:  "session-xyz789",
		},
		{
			name:  "name with dots",
			title: "file.name.here",
			id:    "abc123",
			want:  "filenamehere",
		},
		{
			name:  "very long name preserved as-is",
			title: strings.Repeat("a", 300),
			id:    "abc123",
			want:  strings.Repeat("a", 300),
		},
		{
			name:  "underscores and hyphens preserved",
			title: "my_chat-session",
			id:    "abc123",
			want:  "my_chat-session",
		},
		{
			name:  "leading and trailing hyphens trimmed",
			title: " --hello-- ",
			id:    "abc123",
			want:  "hello",
		},
		{
			name:  "all special chars result in fallback",
			title: "!@#$%^&*()",
			id:    "abc123",
			want:  "session-abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SanitizeExportFilename(tt.title, tt.id)
			if got != tt.want {
				t.Errorf("SanitizeExportFilename(%q, %q) = %q, want %q", tt.title, tt.id, got, tt.want)
			}
		})
	}
}

func TestSampleIDs(t *testing.T) {
	tests := []struct {
		name string
		ids  []string
		n    int
		want []string
	}{
		{
			name: "empty slice returns nil",
			ids:  nil,
			n:    5,
			want: nil,
		},
		{
			name: "empty slice non-nil returns nil",
			ids:  []string{},
			n:    5,
			want: nil,
		},
		{
			name: "n zero returns nil",
			ids:  []string{"a", "b"},
			n:    0,
			want: nil,
		},
		{
			name: "n negative returns nil",
			ids:  []string{"a", "b"},
			n:    -1,
			want: nil,
		},
		{
			name: "slice smaller than n returns copy of all",
			ids:  []string{"a", "b"},
			n:    5,
			want: []string{"a", "b"},
		},
		{
			name: "slice equal to n returns copy",
			ids:  []string{"a", "b", "c"},
			n:    3,
			want: []string{"a", "b", "c"},
		},
		{
			name: "slice larger than n returns first n",
			ids:  []string{"a", "b", "c", "d", "e"},
			n:    3,
			want: []string{"a", "b", "c"},
		},
		{
			name: "single element slice with n=1",
			ids:  []string{"only"},
			n:    1,
			want: []string{"only"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SampleIDs(tt.ids, tt.n)
			if len(got) != len(tt.want) {
				t.Errorf("SampleIDs(%v, %d) = %v, want %v", tt.ids, tt.n, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("SampleIDs(%v, %d) = %v, want %v", tt.ids, tt.n, got, tt.want)
					return
				}
			}
			if tt.want != nil && len(tt.ids) <= tt.n {
				if &got[0] == &tt.ids[0] {
					t.Errorf("SampleIDs should return a copy, not alias the original slice")
				}
			}
		})
	}
}

func TestScopeToSearchQuery(t *testing.T) {
	tests := []struct {
		name   string
		scope  SessionBatchScope
		limit  int
		offset int
		want   SessionSearchQuery
	}{
		{
			name: "all fields mapped",
			scope: SessionBatchScope{
				OwnerType:     "agent",
				AgentID:       "agent-1",
				TeamID:        "team-1",
				Status:        "active",
				ContextStatus: "idle",
				Keyword:       "hello",
				UserID:        "user-1",
			},
			limit:  50,
			offset: 10,
			want: SessionSearchQuery{
				OwnerType:     "agent",
				AgentID:       "agent-1",
				TeamID:        "team-1",
				Status:        "active",
				ContextStatus: "idle",
				Keyword:       "hello",
				UserID:        "user-1",
				Limit:         50,
				Offset:        10,
			},
		},
		{
			name:   "zero limit defaults to SessionBatchPageSize",
			scope:  SessionBatchScope{},
			limit:  0,
			offset: 0,
			want: SessionSearchQuery{
				Limit:  SessionBatchPageSize,
				Offset: 0,
			},
		},
		{
			name:   "negative limit defaults to SessionBatchPageSize",
			scope:  SessionBatchScope{},
			limit:  -1,
			offset: 0,
			want: SessionSearchQuery{
				Limit:  SessionBatchPageSize,
				Offset: 0,
			},
		},
		{
			name:   "negative offset defaults to zero",
			scope:  SessionBatchScope{},
			limit:  100,
			offset: -5,
			want: SessionSearchQuery{
				Limit:  100,
				Offset: 0,
			},
		},
		{
			name: "partial scope fields",
			scope: SessionBatchScope{
				AgentID: "agent-2",
				Status:  "archived",
			},
			limit:  200,
			offset: 50,
			want: SessionSearchQuery{
				AgentID: "agent-2",
				Status:  "archived",
				Limit:   200,
				Offset:  50,
			},
		},
		{
			name: "empty scope with valid limit and offset",
			scope: SessionBatchScope{
				Keyword: "search term",
			},
			limit:  25,
			offset: 100,
			want: SessionSearchQuery{
				Keyword: "search term",
				Limit:   25,
				Offset:  100,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ScopeToSearchQuery(tt.scope, tt.limit, tt.offset)
			if got != tt.want {
				t.Errorf("ScopeToSearchQuery(%+v, %d, %d) = %+v, want %+v", tt.scope, tt.limit, tt.offset, got, tt.want)
			}
		})
	}
}
