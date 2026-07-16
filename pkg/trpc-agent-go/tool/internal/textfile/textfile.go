//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package textfile provides shared text-file encode/decode helpers for
// file and claudecode toolsets (encoding, line endings, quote fuzzy match).
package textfile

import (
	"bytes"
	"regexp"
	"strings"
	stdunicode "unicode"

	textunicode "golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

// DetectLineEnding returns "\r\n" when CRLF is present, otherwise "\n".
func DetectLineEnding(raw []byte) string {
	if bytes.Contains(raw, []byte("\r\n")) {
		return "\r\n"
	}
	return "\n"
}

// NormalizeNewlines converts CRLF/CR to LF for in-memory editing.
func NormalizeNewlines(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return content
}

func applyLineEnding(content string, lineEnding string) string {
	if lineEnding == "\r\n" {
		return strings.ReplaceAll(content, "\n", "\r\n")
	}
	return content
}

// DecodeBytes decodes file bytes to a normalized (LF) string and encoding name.
func DecodeBytes(raw []byte) (content string, encoding string, err error) {
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

// EncodeBytes encodes content using encoding and lineEnding for disk write.
func EncodeBytes(content string, encoding string, lineEnding string) ([]byte, error) {
	normalized := applyLineEnding(content, lineEnding)
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

// IsProbablyBinary reports whether raw looks like binary (NUL bytes),
// excluding UTF-16LE BOM text.
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

// SplitLines splits LF-normalized content into lines without trailing empty
// segment caused by a final newline.
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

// NormalizeQuotes maps curly quotes to ASCII for fuzzy search.
func NormalizeQuotes(raw string) string {
	replacer := strings.NewReplacer(
		"‘", "'",
		"’", "'",
		"“", "\"",
		"”", "\"",
	)
	return replacer.Replace(raw)
}

// FindActualString finds searchString in fileContent, allowing curly-quote
// variants. Returns the matched substring from the file, or "".
func FindActualString(fileContent string, searchString string) string {
	if strings.Contains(fileContent, searchString) {
		return searchString
	}
	var builder strings.Builder
	for _, r := range searchString {
		switch r {
		case '\'':
			builder.WriteString("['‘’]")
		case '"':
			builder.WriteString("[\"“”]")
		default:
			builder.WriteString(regexp.QuoteMeta(string(r)))
		}
	}
	re, err := regexp.Compile(builder.String())
	if err != nil {
		return ""
	}
	return re.FindString(fileContent)
}

// PreserveQuoteStyle adapts newString quotes to match actualOldString style.
func PreserveQuoteStyle(oldString string, actualOldString string, newString string) string {
	if oldString == actualOldString {
		return newString
	}
	hasDoubleQuotes := strings.Contains(actualOldString, "“") || strings.Contains(actualOldString, "”")
	hasSingleQuotes := strings.Contains(actualOldString, "‘") || strings.Contains(actualOldString, "’")
	result := newString
	if hasDoubleQuotes {
		result = applyCurlyDoubleQuotes(result)
	}
	if hasSingleQuotes {
		result = applyCurlySingleQuotes(result)
	}
	return result
}

func applyCurlyDoubleQuotes(raw string) string {
	chars := []rune(raw)
	out := make([]rune, 0, len(chars))
	for idx, r := range chars {
		if r != '"' {
			out = append(out, r)
			continue
		}
		if isOpeningQuote(chars, idx) {
			out = append(out, '“')
			continue
		}
		out = append(out, '”')
	}
	return string(out)
}

func applyCurlySingleQuotes(raw string) string {
	chars := []rune(raw)
	out := make([]rune, 0, len(chars))
	for idx, r := range chars {
		if r != '\'' {
			out = append(out, r)
			continue
		}
		prevIsLetter := idx > 0 && stdunicode.IsLetter(chars[idx-1])
		nextIsLetter := idx+1 < len(chars) && stdunicode.IsLetter(chars[idx+1])
		if prevIsLetter && nextIsLetter {
			out = append(out, '’')
			continue
		}
		if isOpeningQuote(chars, idx) {
			out = append(out, '‘')
			continue
		}
		out = append(out, '’')
	}
	return string(out)
}

func isOpeningQuote(chars []rune, idx int) bool {
	if idx == 0 {
		return true
	}
	prev := chars[idx-1]
	return stdunicode.IsSpace(prev) || strings.ContainsRune("([{", prev)
}
