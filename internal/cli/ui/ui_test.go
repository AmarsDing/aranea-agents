package ui

import (
	"bytes"
	"os"
	"testing"
)

func TestDetect_NoColorEnv(t *testing.T) {
	os.Setenv("NO_COLOR", "1")
	defer os.Unsetenv("NO_COLOR")
	u := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if !u.NoColor {
		t.Fatal("expected NoColor=true from NO_COLOR env")
	}
}

func TestDetect_AraneaNoColorEnv(t *testing.T) {
	os.Setenv("ARANEA_NO_COLOR", "1")
	defer os.Unsetenv("ARANEA_NO_COLOR")
	u := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if !u.NoColor {
		t.Fatal("expected NoColor=true from ARANEA_NO_COLOR env")
	}
}

func TestDetect_NoColorFlag(t *testing.T) {
	u := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, true)
	if !u.NoColor {
		t.Fatal("expected NoColor=true from flag")
	}
}

func TestDetect_DefaultWidth(t *testing.T) {
	u := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if u.Width <= 0 {
		t.Fatalf("expected positive width, got %d", u.Width)
	}
}

func TestDetect_NonTTYDefaultWidth(t *testing.T) {
	u := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if u.Width != 80 {
		t.Fatalf("non-TTY should default to 80, got %d", u.Width)
	}
}

func TestDetect_NonTTY(t *testing.T) {
	u := Detect(nil, &bytes.Buffer{}, &bytes.Buffer{}, false)
	if u.IsTTY {
		t.Fatal("bytes.Buffer should not be a TTY")
	}
}

func TestColor_NoColor(t *testing.T) {
	u := UI{NoColor: true}
	fn := u.Color("red")
	result := fn("hello")
	if result != "hello" {
		t.Fatalf("NoColor mode should return plain string, got %q", result)
	}
}

func TestColor_UnknownName(t *testing.T) {
	u := UI{}
	fn := u.Color("unknown")
	result := fn("hello")
	if result != "hello" {
		t.Fatalf("unknown color should use fmt.Sprintf, got %q", result)
	}
}

func TestColor_KnownNames(t *testing.T) {
	u := UI{}
	for _, name := range []string{"red", "yellow", "green", "dim", "bold", "cyan"} {
		fn := u.Color(name)
		if fn == nil {
			t.Fatalf("Color(%q) returned nil", name)
		}
	}
}
