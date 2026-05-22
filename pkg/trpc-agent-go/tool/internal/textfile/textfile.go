//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

// Package textfile provides pure text encoding, line-ending, and fuzzy-match
// helpers shared by file tools and claudecode.
package textfile

import (
	"bytes"
	"strings"

	textunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// SplitLines splits content on newlines, omitting the trailing empty segment
// when content ends with a newline.
func SplitLines(content string) []string {
	if content == "" {
		return []string{}
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		return lines[:len(lines)-1]
	}
	return lines
}

// NormalizeNewlines converts CRLF and CR to LF.
func NormalizeNewlines(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

// DetectLineEnding returns "\r\n" when raw contains CRLF, else "\n".
func DetectLineEnding(raw []byte) string {
	if bytes.Contains(raw, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// ApplyLineEnding converts LF in content to the requested line ending.
func ApplyLineEnding(content string, lineEnding string) string {
	if lineEnding == "\r\n" {
		return strings.ReplaceAll(content, "\n", "\r\n")
	}
	return content
}

// DecodeBytes decodes raw file bytes to normalized UTF-8 text and reports encoding.
func DecodeBytes(raw []byte) (string, string, error) {
	if len(raw) >= 2 && raw[0] == 0xff && raw[1] == 0xfe {
		decoder := textunicode.UTF16(textunicode.LittleEndian, textunicode.ExpectBOM).NewDecoder()
		decoded, _, err := transform.String(decoder, string(raw))
		if err != nil {
			return "", "", err
		}
		return NormalizeNewlines(decoded), "utf16le", nil
	}
	return NormalizeNewlines(string(raw)), "utf8", nil
}

// EncodeBytes encodes text for writing with the given encoding and line ending.
func EncodeBytes(content string, encoding string, lineEnding string) ([]byte, error) {
	normalized := ApplyLineEnding(content, lineEnding)
	if encoding == "utf16le" {
		encoder := textunicode.UTF16(textunicode.LittleEndian, textunicode.UseBOM).NewEncoder()
		encoded, _, err := transform.String(encoder, normalized)
		if err != nil {
			return nil, err
		}
		return []byte(encoded), nil
	}
	return []byte(normalized), nil
}

// IsProbablyBinary reports whether raw looks like a binary file.
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
