//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package claudecode

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	stdunicode "unicode"

	"golang.org/x/net/html"
	"trpc.group/trpc-go/trpc-agent-go/tool/file/patch"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

func normalizePath(baseDir string, raw string) (string, string, error) {
	pathValue := strings.TrimSpace(raw)
	if pathValue == "" {
		return "", "", fmt.Errorf("path is required")
	}
	cleanBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", "", err
	}
	if filepath.IsAbs(pathValue) {
		cleanPath := filepath.Clean(pathValue)
		rel, err := filepath.Rel(cleanBase, cleanPath)
		if err != nil {
			return "", "", err
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", "", fmt.Errorf("path is outside base_dir: %s", raw)
		}
		return filepath.ToSlash(filepath.Clean(rel)), cleanPath, nil
	}
	cleanPath := filepath.Clean(pathValue)
	if cleanPath == ".." || strings.HasPrefix(cleanPath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("path is outside base_dir: %s", raw)
	}
	absPath := filepath.Join(cleanBase, cleanPath)
	return filepath.ToSlash(filepath.Clean(cleanPath)), absPath, nil
}

func (r *runtime) currentBaseDir() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.baseDir
}

func (r *runtime) setBaseDir(baseDir string) {
	r.mu.Lock()
	r.baseDir = baseDir
	r.mu.Unlock()
}

func relativePath(baseDir string, absPath string) string {
	baseAbs, err := filepath.Abs(baseDir)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(absPath))
	}
	rel, err := filepath.Rel(baseAbs, absPath)
	if err != nil {
		return filepath.ToSlash(filepath.Clean(absPath))
	}
	return filepath.ToSlash(filepath.Clean(rel))
}

func readHTTPBody(
	resp *http.Response,
	maxContentLength int,
	maxTotalContentLength int,
) ([]byte, error) {
	if resp == nil || resp.Body == nil {
		return nil, nil
	}
	limit := maxContentLength
	if maxTotalContentLength > 0 && (limit == 0 || maxTotalContentLength < limit) {
		limit = maxTotalContentLength
	}
	if limit <= 0 {
		limit = 1 << 20
	}
	reader := io.LimitReader(resp.Body, int64(limit)+1)
	body, err := io.ReadAll(reader)
	if err != nil {
		return nil, err
	}
	if len(body) > limit {
		return nil, fmt.Errorf("response body exceeded limit of %d bytes", limit)
	}
	return body, nil
}

func countLines(content string) int {
	if content == "" {
		return 0
	}
	parts := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		return len(parts) - 1
	}
	return len(parts)
}

func splitTextLines(content string) []string {
	return textfile.SplitLines(content)
}

func sliceLines(content string, offset int, limit *int) (string, int, int) {
	lines := splitTextLines(content)
	totalLines := len(lines)
	startLine := offset
	if startLine <= 0 {
		startLine = 1
	}
	startIdx := startLine - 1
	if startIdx > totalLines {
		startIdx = totalLines
	}
	endIdx := totalLines
	if limit != nil && *limit >= 0 && startIdx+*limit < endIdx {
		endIdx = startIdx + *limit
	}
	sliced := lines[startIdx:endIdx]
	result := strings.Join(sliced, "\n")
	if len(sliced) > 0 && strings.HasSuffix(content, "\n") && endIdx == totalLines {
		result += "\n"
	}
	return result, startLine, totalLines
}

func normalizeNewlines(content string) string {
	return textfile.NormalizeNewlines(content)
}

func detectLineEnding(raw []byte) string {
	return textfile.DetectLineEnding(raw)
}

func applyLineEnding(content string, lineEnding string) string {
	return textfile.ApplyLineEnding(content, lineEnding)
}

func decodeTextBytes(raw []byte) (string, string, error) {
	return textfile.DecodeBytes(raw)
}

func encodeTextBytes(content string, encoding string, lineEnding string) ([]byte, error) {
	return textfile.EncodeBytes(content, encoding, lineEnding)
}

func fileBase64(raw []byte) string {
	return base64.StdEncoding.EncodeToString(raw)
}

func isProbablyBinary(raw []byte) bool {
	return textfile.IsProbablyBinary(raw)
}

func buildStructuredPatch(oldContent string, newContent string) []patchHunk {
	return patch.BuildStructured(oldContent, newContent)
}

func matchSearchDomainFilters(
	rawURL string,
	allowed []string,
	blocked []string,
) bool {
	host := searchURLHost(rawURL)
	if host == "" {
		return len(allowed) == 0
	}
	for _, rule := range blocked {
		if matchDomainRule(host, rule) {
			return false
		}
	}
	if len(allowed) == 0 {
		return true
	}
	for _, rule := range allowed {
		if matchDomainRule(host, rule) {
			return true
		}
	}
	return false
}

func searchURLHost(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func matchDomainRule(host string, rule string) bool {
	cleanRule := strings.ToLower(strings.TrimSpace(rule))
	if cleanRule == "" {
		return false
	}
	if strings.HasPrefix(cleanRule, "*.") {
		suffix := strings.TrimPrefix(cleanRule, "*.")
		return host == suffix || strings.HasSuffix(host, "."+suffix)
	}
	return host == cleanRule || strings.HasSuffix(host, "."+cleanRule)
}

func extractHTMLText(raw []byte) string {
	doc, err := html.Parse(bytes.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(string(raw))
	}
	parts := make([]string, 0, 32)
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode {
			name := strings.ToLower(node.Data)
			if name == "script" || name == "style" || name == "noscript" {
				return
			}
		}
		if node.Type == html.TextNode {
			text := strings.TrimSpace(node.Data)
			if text != "" {
				parts = append(parts, collapseWhitespace(text))
			}
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(doc)
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func collapseWhitespace(raw string) string {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return stdunicode.IsSpace(r)
	})
	return strings.Join(fields, " ")
}

func joinOutput(stdout string, stderr string) string {
	switch {
	case stdout == "":
		return stderr
	case stderr == "":
		return stdout
	default:
		return stdout + "\n" + stderr
	}
}

func sortedCopy(items []string) []string {
	out := append([]string{}, items...)
	slices.Sort(out)
	return out
}
