package custom

import (
	"context"
	"testing"
)

func TestNewDemoTool_ReturnsNonNil(t *testing.T) {
	tool := NewDemoTool()
	if tool == nil {
		t.Fatal("NewDemoTool() returned nil")
	}
	d := tool.Declaration()
	if d == nil {
		t.Fatal("Declaration() returned nil")
	}
	if d.Name != "demo_search" {
		t.Fatalf("Name = %q, want %q", d.Name, "demo_search")
	}
	if d.Description == "" {
		t.Fatal("Description should not be empty")
	}
}

func TestDemoExecute_DefaultLimit(t *testing.T) {
	out, err := DemoExecute(context.Background(), DemoInput{Query: "golang", Limit: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 5 {
		t.Fatalf("len(Results) = %d, want 5", len(out.Results))
	}
}

func TestDemoExecute_NegativeLimit(t *testing.T) {
	out, err := DemoExecute(context.Background(), DemoInput{Query: "golang", Limit: -3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 5 {
		t.Fatalf("len(Results) = %d, want 5 (default for negative)", len(out.Results))
	}
}

func TestDemoExecute_CustomLimit(t *testing.T) {
	out, err := DemoExecute(context.Background(), DemoInput{Query: "golang", Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 3 {
		t.Fatalf("len(Results) = %d, want 3", len(out.Results))
	}
}

func TestDemoExecute_ResultFormat(t *testing.T) {
	out, err := DemoExecute(context.Background(), DemoInput{Query: "test", Limit: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(out.Results) != 2 {
		t.Fatalf("len(Results) = %d, want 2", len(out.Results))
	}
	r0 := out.Results[0]
	if r0.Title == "" {
		t.Fatal("Title should not be empty")
	}
	if r0.URL == "" {
		t.Fatal("URL should not be empty")
	}
	if r0.Title != `Result 1 for "test"` {
		t.Fatalf("Title = %q, want %q", r0.Title, `Result 1 for "test"`)
	}
	if r0.URL != "https://example.com/result/1" {
		t.Fatalf("URL = %q, want %q", r0.URL, "https://example.com/result/1")
	}
	r1 := out.Results[1]
	if r1.Title != `Result 2 for "test"` {
		t.Fatalf("Title = %q, want %q", r1.Title, `Result 2 for "test"`)
	}
	if r1.URL != "https://example.com/result/2" {
		t.Fatalf("URL = %q, want %q", r1.URL, "https://example.com/result/2")
	}
}
