package shared

import (
	"testing"

	"github.com/go-kratos/kratos/v2/errors"
	"go.einride.tech/aip/filtering"
	"go.einride.tech/aip/ordering"
)

func TestPageToLimitOffset(t *testing.T) {
	tests := []struct {
		name            string
		page            int32
		pageSize        int32
		wantLimit       int
		wantOffset      int
		wantPageOut     int32
		wantPageSizeOut int32
	}{
		{"normal first page", 1, 20, 20, 0, 1, 20},
		{"zero values default", 0, 0, 20, 0, 1, 20},
		{"second page large size", 2, 50, 50, 50, 2, 50},
		{"negative values default", -1, -1, 20, 0, 1, 20},
		{"page size capped at 100", 1, 200, 100, 0, 1, 100},
		{"third page small size", 3, 10, 10, 20, 3, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotLimit, gotOffset, gotPageOut, gotPageSizeOut := PageToLimitOffset(tt.page, tt.pageSize)
			if gotLimit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", gotLimit, tt.wantLimit)
			}
			if gotOffset != tt.wantOffset {
				t.Errorf("offset = %d, want %d", gotOffset, tt.wantOffset)
			}
			if gotPageOut != tt.wantPageOut {
				t.Errorf("pageOut = %d, want %d", gotPageOut, tt.wantPageOut)
			}
			if gotPageSizeOut != tt.wantPageSizeOut {
				t.Errorf("pageSizeOut = %d, want %d", gotPageSizeOut, tt.wantPageSizeOut)
			}
		})
	}
}

func TestListFilter(t *testing.T) {
	f := filtering.Filter{}
	opts := &ListOptions{}
	ListFilter(f)(opts)
	if opts.Filter != f {
		t.Errorf("Filter = %v, want %v", opts.Filter, f)
	}
}

func TestListOrderBy(t *testing.T) {
	ob := ordering.OrderBy{Fields: []ordering.Field{{Path: "name"}}}
	opts := &ListOptions{}
	ListOrderBy(ob)(opts)
	if len(opts.OrderBy.Fields) != len(ob.Fields) {
		t.Errorf("OrderBy.Fields length = %d, want %d", len(opts.OrderBy.Fields), len(ob.Fields))
	}
	if len(opts.OrderBy.Fields) > 0 && opts.OrderBy.Fields[0].Path != ob.Fields[0].Path {
		t.Errorf("OrderBy.Fields[0].Path = %q, want %q", opts.OrderBy.Fields[0].Path, ob.Fields[0].Path)
	}
}

func TestListOffset(t *testing.T) {
	opts := &ListOptions{}
	ListOffset(42)(opts)
	if opts.Offset != 42 {
		t.Errorf("Offset = %d, want 42", opts.Offset)
	}
}

func TestListLimit(t *testing.T) {
	opts := &ListOptions{}
	ListLimit(10)(opts)
	if opts.Limit != 10 {
		t.Errorf("Limit = %d, want 10", opts.Limit)
	}
}

func TestListOptionsCombined(t *testing.T) {
	f := filtering.Filter{}
	ob := ordering.OrderBy{Fields: []ordering.Field{{Path: "created_at"}}}
	opts := &ListOptions{}
	ListFilter(f)(opts)
	ListOrderBy(ob)(opts)
	ListOffset(100)(opts)
	ListLimit(25)(opts)

	if opts.Filter != f {
		t.Errorf("Filter = %v, want %v", opts.Filter, f)
	}
	if len(opts.OrderBy.Fields) != 1 || opts.OrderBy.Fields[0].Path != "created_at" {
		t.Errorf("OrderBy not applied correctly")
	}
	if opts.Offset != 100 {
		t.Errorf("Offset = %d, want 100", opts.Offset)
	}
	if opts.Limit != 25 {
		t.Errorf("Limit = %d, want 25", opts.Limit)
	}
}

func TestJSONStringList(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    []string
		wantErr bool
	}{
		{"empty string", "", nil, false},
		{"empty array", "[]", nil, false},
		{"valid string array", `["a","b"]`, []string{"a", "b"}, false},
		{"empty object returns nil", "{}", nil, false},
		{"invalid json", "not json", nil, true},
		{"mixed types", `["a",1]`, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JSONStringList(tt.raw)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantErr {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("len = %d, want %d", len(got), len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestValidateDocumentAgainstSchema(t *testing.T) {
	tests := []struct {
		name      string
		module    string
		schema    string
		doc       string
		wantErr   bool
		errReason string
	}{
		{
			"valid doc against schema",
			"TEST",
			`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			`{"name":"hello"}`,
			false,
			"",
		},
		{
			"invalid doc against schema",
			"TEST",
			`{"type":"object","properties":{"name":{"type":"string"}},"required":["name"]}`,
			`{"age":1}`,
			true,
			"TEST",
		},
		{
			"empty schema passes",
			"TEST",
			"",
			`{"anything":1}`,
			false,
			"",
		},
		{
			"empty object schema passes",
			"TEST",
			"{}",
			`{"anything":1}`,
			false,
			"",
		},
		{
			"empty doc defaults to empty object",
			"TEST",
			`{"type":"object","properties":{"name":{"type":"string"}}}`,
			"",
			false,
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateDocumentAgainstSchema(tt.module, tt.schema, tt.doc)
			if (err != nil) != tt.wantErr {
				t.Errorf("err = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil {
					if e.Reason != tt.errReason {
						t.Errorf("err reason = %q, want %q", e.Reason, tt.errReason)
					}
				} else {
					t.Errorf("expected kratos error, got %T", err)
				}
			}
		})
	}
}
