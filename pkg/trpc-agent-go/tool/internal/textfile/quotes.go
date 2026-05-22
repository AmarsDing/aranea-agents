//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package textfile

import (
	"regexp"
	"strings"
	"unicode"
)

// NormalizeQuotes replaces curly quotes with ASCII quotes.
func NormalizeQuotes(raw string) string {
	replacer := strings.NewReplacer(
		"‘", "'",
		"’", "'",
		"“", "\"",
		"”", "\"",
	)
	return replacer.Replace(raw)
}

// ApplyCurlySingleQuotes converts ASCII single quotes to curly variants.
func ApplyCurlySingleQuotes(raw string) string {
	return applyCurlySingleQuotes(raw)
}

// FindActualString locates searchString in fileContent, allowing curly quote variants.
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

// PreserveQuoteStyle adjusts newString quote characters to match actualOldString style.
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
		prevIsLetter := idx > 0 && unicode.IsLetter(chars[idx-1])
		nextIsLetter := idx+1 < len(chars) && unicode.IsLetter(chars[idx+1])
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
	return unicode.IsSpace(prev) || strings.ContainsRune("([{", prev)
}
