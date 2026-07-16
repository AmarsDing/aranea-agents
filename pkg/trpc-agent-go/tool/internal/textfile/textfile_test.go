//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package textfile

import "testing"

func TestFindActualString_QuoteFuzzy(t *testing.T) {
	content := "say “hello”\n"
	got := FindActualString(content, `say "hello"`)
	if got != "say “hello”" {
		t.Fatalf("got %q", got)
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	raw, err := EncodeBytes("a\nb\n", "utf8", "\r\n")
	if err != nil {
		t.Fatal(err)
	}
	content, enc, err := DecodeBytes(raw)
	if err != nil {
		t.Fatal(err)
	}
	if enc != "utf8" || content != "a\nb\n" {
		t.Fatalf("content=%q enc=%s", content, enc)
	}
	if DetectLineEnding(raw) != "\r\n" {
		t.Fatal("expected CRLF")
	}
}
