package orgimport

import "testing"

func TestIsMDFile_MdExtension(t *testing.T) {
	if !isMDFile("file.md") {
		t.Error("expected true for .md extension")
	}
}

func TestIsMDFile_MarkdownExtension(t *testing.T) {
	if !isMDFile("file.markdown") {
		t.Error("expected true for .markdown extension")
	}
}

func TestIsMDFile_MdUpperCase(t *testing.T) {
	if !isMDFile("FILE.MD") {
		t.Error("expected true for .MD extension (case insensitive)")
	}
}

func TestIsMDFile_TxtExtension(t *testing.T) {
	if isMDFile("file.txt") {
		t.Error("expected false for .txt extension")
	}
}

func TestIsMDFile_NoExtension(t *testing.T) {
	if isMDFile("file") {
		t.Error("expected false for no extension")
	}
}

func TestIsMDFile_HiddenMdFile(t *testing.T) {
	if !isMDFile(".md") {
		t.Error("expected true for .md hidden file")
	}
}

func TestStripCodeFence_TripleBacktick(t *testing.T) {
	input := "```json\n{\"key\":\"value\"}\n```"
	want := "{\"key\":\"value\"}"
	got := stripCodeFence(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_TripleTilde(t *testing.T) {
	input := "~~~\n{\"key\":\"value\"}\n~~~"
	want := "~~~\n{\"key\":\"value\"}\n~~~"
	got := stripCodeFence(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_NoFenceInput(t *testing.T) {
	input := "{\"key\":\"value\"}"
	want := "{\"key\":\"value\"}"
	got := stripCodeFence(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestStripCodeFence_NestedFence(t *testing.T) {
	input := "```json\n```inner\ncontent\n```\n```"
	want := "```inner\ncontent\n```"
	got := stripCodeFence(input)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
