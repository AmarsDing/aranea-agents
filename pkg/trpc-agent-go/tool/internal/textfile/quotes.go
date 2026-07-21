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

import (
	"regexp"
	"strings"
	"unicode"
)

// NormalizeQuotes replaces curly quotes with their straight
// equivalents.
func NormalizeQuotes(raw string) string {
	replacer := strings.NewReplacer(
		"‘", "'",
		"’", "'",
		"“", "\"",
		"”", "\"",
	)
	return replacer.Replace(raw)
}

// FindActualString locates searchString inside fileContent, tolerating
// curly vs straight quote differences. It returns the actual matched
// text from fileContent, or "" when there is no match.
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

// PreserveQuoteStyle rewrites newString so its quotes match the style
// found in actualOldString (the text actually present in the file).
func PreserveQuoteStyle(
	oldString string,
	actualOldString string,
	newString string,
) string {
	if oldString == actualOldString {
		return newString
	}
	hasDoubleQuotes := strings.Contains(actualOldString, "“") ||
		strings.Contains(actualOldString, "”")
	hasSingleQuotes := strings.Contains(actualOldString, "‘") ||
		strings.Contains(actualOldString, "’")
	result := newString
	if hasDoubleQuotes {
		result = ApplyCurlyDoubleQuotes(result)
	}
	if hasSingleQuotes {
		result = ApplyCurlySingleQuotes(result)
	}
	return result
}

// ApplyCurlyDoubleQuotes converts straight double quotes to curly
// quotes using opening/closing heuristics.
func ApplyCurlyDoubleQuotes(raw string) string {
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

// ApplyCurlySingleQuotes converts straight single quotes to curly
// quotes using opening/closing and apostrophe heuristics.
func ApplyCurlySingleQuotes(raw string) string {
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
