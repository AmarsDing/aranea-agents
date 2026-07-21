//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package textfile provides shared text file helpers for editing tools:
// encoding detection, line-ending normalization, and quote-fuzzy
// matching. It is shared by tool/file and tool/claudecode.
package textfile

import (
	"bytes"
	"strings"

	textunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// DetectLineEnding returns "\r\n" when the raw bytes contain CRLF
// sequences, otherwise "\n".
func DetectLineEnding(raw []byte) string {
	if bytes.Contains(raw, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// ApplyLineEnding converts LF newlines in content to CRLF when
// lineEnding is "\r\n"; otherwise content is returned unchanged.
func ApplyLineEnding(content string, lineEnding string) string {
	if lineEnding == "\r\n" {
		return strings.ReplaceAll(content, "\n", "\r\n")
	}
	return content
}

// NormalizeNewlines converts CRLF and lone CR newlines to LF.
func NormalizeNewlines(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

// CountLines returns the number of lines in content. A trailing
// newline does not create an extra empty line.
func CountLines(content string) int {
	if content == "" {
		return 0
	}
	parts := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		return len(parts) - 1
	}
	return len(parts)
}

// SplitTextLines splits content on LF. A trailing newline does not
// produce a trailing empty element.
func SplitTextLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		return lines[:len(lines)-1]
	}
	return lines
}

// DecodeTextBytes decodes raw bytes to normalized text. UTF-16LE with
// BOM is decoded via the BOM; everything else is treated as UTF-8.
// Returned content always has LF newlines.
func DecodeTextBytes(raw []byte) (string, string, error) {
	if len(raw) >= 2 && raw[0] == 0xff && raw[1] == 0xfe {
		decoder := textunicode.UTF16(
			textunicode.LittleEndian,
			textunicode.ExpectBOM,
		).NewDecoder()
		decoded, _, err := transform.String(decoder, string(raw))
		if err != nil {
			return "", "", err
		}
		return NormalizeNewlines(decoded), "utf16le", nil
	}
	return NormalizeNewlines(string(raw)), "utf8", nil
}

// EncodeTextBytes encodes content using the given encoding and line
// ending. UTF-16LE output includes a BOM.
func EncodeTextBytes(
	content string,
	encoding string,
	lineEnding string,
) ([]byte, error) {
	normalized := ApplyLineEnding(content, lineEnding)
	if encoding == "utf16le" {
		encoder := textunicode.UTF16(
			textunicode.LittleEndian,
			textunicode.UseBOM,
		).NewEncoder()
		encoded, _, err := transform.String(encoder, normalized)
		if err != nil {
			return nil, err
		}
		return []byte(encoded), nil
	}
	return []byte(normalized), nil
}

// IsProbablyBinary reports whether raw looks like binary content.
// UTF-16LE BOM content is treated as text.
func IsProbablyBinary(raw []byte) bool {
	if len(raw) >= 2 && raw[0] == 0xff && raw[1] == 0xfe {
		return false
	}
	for _, b := range raw {
		if b == 0 {
			return true
		}
	}
	return false
}
