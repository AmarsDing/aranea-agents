package rgsearch

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"
)

const (
	maxFiles        = 50
	maxMatchesTotal = 200
	maxLineRunes    = 300
	maxPerFile      = 20
)

type searchRequest struct {
	Path                 string `json:"path"`
	FilePattern          string `json:"file_pattern"`
	FileCaseSensitive    bool   `json:"file_case_sensitive"`
	ContentPattern       string `json:"content_pattern"`
	ContentCaseSensitive bool   `json:"content_case_sensitive"`
	After                int    `json:"after"`
	Before               int    `json:"before"`
	Context              int    `json:"context"`
	Type                 string `json:"type"`
	Multiline            bool   `json:"multiline"`
	HeadLimit            int    `json:"head_limit"`
	Offset               int    `json:"offset"`
}

type lineMatch struct {
	LineNumber  int    `json:"line_number"`
	LineContent string `json:"line_content"`
	Kind        string `json:"kind,omitempty"`
}

type fileMatch struct {
	FilePath string       `json:"file_path"`
	Matches  []*lineMatch `json:"matches"`
	Message  string       `json:"message"`
}

type searchResponse struct {
	BaseDirectory  string       `json:"base_directory"`
	Path           string       `json:"path"`
	FilePattern    string       `json:"file_pattern"`
	ContentPattern string       `json:"content_pattern"`
	FileMatches    []*fileMatch `json:"file_matches"`
	Message        string       `json:"message"`
	Engine         string       `json:"engine,omitempty"`
	Truncated      bool         `json:"truncated,omitempty"`
}

type rgJSONEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
	} `json:"data"`
}

func parseRipgrepJSON(stdout, baseDir, relRoot string) (files []*fileMatch, truncated bool) {
	byPath := map[string]*fileMatch{}
	order := make([]string, 0, 16)
	total := 0
	for _, line := range strings.Split(stdout, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev rgJSONEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		kind := lineKind(ev.Type)
		if kind == "" {
			continue
		}
		path := strings.TrimSpace(ev.Data.Path.Text)
		if path == "" {
			continue
		}
		path = relativize(path, baseDir, relRoot)
		fm, ok := byPath[path]
		if !ok {
			if len(order) >= maxFiles {
				truncated = true
				continue
			}
			fm = &fileMatch{FilePath: path, Matches: nil}
			byPath[path] = fm
			order = append(order, path)
		}
		if kind == "match" {
			if countMatchLines(fm.Matches) >= maxPerFile || total >= maxMatchesTotal {
				truncated = true
				continue
			}
			total++
		}
		content := strings.TrimRight(ev.Data.Lines.Text, "\r\n")
		content = clipRunes(content, maxLineRunes)
		n := ev.Data.LineNumber
		if n <= 0 {
			n = 1
		}
		fm.Matches = append(fm.Matches, &lineMatch{LineNumber: n, LineContent: content, Kind: kind})
	}
	out := make([]*fileMatch, 0, len(order))
	for _, p := range order {
		fm := byPath[p]
		fm.Message = "Found " + strconv.Itoa(countMatchLines(fm.Matches)) + " matches in file '" + p + "'"
		out = append(out, fm)
	}
	return out, truncated
}

func lineKind(evType string) string {
	switch strings.ToLower(strings.TrimSpace(evType)) {
	case "match":
		return "match"
	case "context":
		return "context"
	default:
		return ""
	}
}

func isMatchLine(lm *lineMatch) bool {
	if lm == nil {
		return false
	}
	return lm.Kind == "" || lm.Kind == "match"
}

func countMatchLines(in []*lineMatch) int {
	n := 0
	for _, lm := range in {
		if isMatchLine(lm) {
			n++
		}
	}
	return n
}

func paginateFileMatches(files []*fileMatch, offset, headLimit int) ([]*fileMatch, bool) {
	if offset < 0 {
		offset = 0
	}
	limitMatches := headLimit
	if limitMatches <= 0 {
		if offset <= 0 {
			return files, false
		}
		limitMatches = maxMatchesTotal
	}
	matchSeen := 0
	truncated := false
	out := make([]*fileMatch, 0, len(files))
	for _, fm := range files {
		if fm == nil {
			continue
		}
		kept := make([]*lineMatch, 0, len(fm.Matches))
		pending := []*lineMatch{}
		lastKept := false
		for _, lm := range fm.Matches {
			if lm == nil {
				continue
			}
			if !isMatchLine(lm) {
				if lastKept {
					kept = append(kept, lm)
				} else if matchSeen >= offset {
					pending = append(pending, lm)
				}
				continue
			}
			matchSeen++
			if matchSeen <= offset {
				pending = nil
				lastKept = false
				continue
			}
			if matchSeen-offset > limitMatches {
				truncated = true
				pending = nil
				lastKept = false
				continue
			}
			kept = append(kept, pending...)
			pending = nil
			kept = append(kept, lm)
			lastKept = true
		}
		if len(kept) == 0 {
			continue
		}
		cp := *fm
		cp.Matches = kept
		cp.Message = "Found " + strconv.Itoa(countMatchLines(kept)) + " matches in file '" + cp.FilePath + "'"
		out = append(out, &cp)
	}
	if matchSeen > offset+limitMatches {
		truncated = true
	}
	return out, truncated
}

func capFileMatches(in []*fileMatch) (out []*fileMatch, truncated bool) {
	if len(in) == 0 {
		return in, false
	}
	total := 0
	for i, fm := range in {
		if i >= maxFiles {
			return out, true
		}
		if fm == nil {
			continue
		}
		cp := *fm
		if len(cp.Matches) > maxPerFile {
			cp.Matches = cp.Matches[:maxPerFile]
			truncated = true
		}
		remain := maxMatchesTotal - total
		if remain <= 0 {
			return out, true
		}
		if len(cp.Matches) > remain {
			cp.Matches = cp.Matches[:remain]
			truncated = true
		}
		for _, m := range cp.Matches {
			if m != nil {
				m.LineContent = clipRunes(m.LineContent, maxLineRunes)
			}
		}
		total += countMatchLines(cp.Matches)
		cp.Message = "Found " + strconv.Itoa(countMatchLines(cp.Matches)) + " matches in file '" + cp.FilePath + "'"
		out = append(out, &cp)
	}
	return out, truncated
}

func clipRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func relativize(path, baseDir, relRoot string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	baseDir = strings.ReplaceAll(baseDir, "\\", "/")
	if baseDir != "" {
		pre := strings.TrimRight(baseDir, "/") + "/"
		if strings.HasPrefix(strings.ToLower(path), strings.ToLower(pre)) {
			path = path[len(pre):]
		}
	}
	if relRoot != "" && relRoot != "." {
		root := strings.ReplaceAll(strings.Trim(relRoot, "/"), "\\", "/")
		if !strings.HasPrefix(path, root+"/") && path != root {
			path = strings.Trim(root+"/"+path, "/")
		}
	}
	return path
}
