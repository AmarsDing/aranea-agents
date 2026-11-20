package fleet

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 用量统计读本机 agent 自己写的会话日志（不调任何远端接口）：
//   - Claude Code：~/.claude/projects/**/*.jsonl，assistant 消息带 usage；
//   - Codex：~/.codex/sessions/**/rollout-*.jsonl，token_count 事件带用量与额度百分比。
// 5 小时与 7 天两个窗口与两家的额度窗口一致。Claude 日志没有额度信息，百分比为 -1。

type usagePoint struct {
	at     int64
	tokens int64
}

type fileUsage struct {
	size    int64
	modUnix int64
	points  []usagePoint
	limits  map[string]limitInfo // codex: primary/secondary
	limitAt int64
	codex   *codexRateLimits // codex: 同一行快照里的 plan_type 与 credits
}

type limitInfo struct {
	percent   float64
	resetsAt  int64
	windowMin int64
}

// windowLabel 按额度窗口长度命名：300 分钟 = 5h，10080 = 7d，其余按天/小时。
func windowLabel(min int64) string {
	switch {
	case min <= 0:
		return ""
	case min%1440 == 0:
		return fmt.Sprintf("%dd", min/1440)
	case min%60 == 0:
		return fmt.Sprintf("%dh", min/60)
	}
	return fmt.Sprintf("%dm", min)
}

var usageFiles = map[string]*fileUsage{}

const usageTTL = time.Minute

// Usage 返回各 agent 的用量（60 秒缓存）。
func (u *Usecase) Usage() []AgentUsage {
	u.usageMu.Lock()
	defer u.usageMu.Unlock()
	if time.Since(u.usageAt) < usageTTL && u.usageCache != nil {
		return u.usageCache
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	now := time.Now()
	var out []AgentUsage
	if r, ok := scanUsage(filepath.Join(home, ".claude", "projects"), now, parseClaudeLine); ok {
		r.Agent, r.Source = "claude", "~/.claude/projects"
		out = append(out, r)
	}
	if r, ok := scanUsage(filepath.Join(home, ".codex", "sessions"), now, parseCodexLine); ok {
		r.Agent, r.Source = "codex", "~/.codex/sessions"
		out = append(out, r)
	}
	u.usageCache, u.usageAt = out, now
	return out
}

type lineParser func(line []byte, seen map[string]bool) (usagePoint, map[string]limitInfo, bool)

func scanUsage(root string, now time.Time, parse lineParser) (AgentUsage, bool) {
	if _, err := os.Stat(root); err != nil {
		return AgentUsage{}, false
	}
	cutoff := now.Add(-7 * 24 * time.Hour)
	h5 := now.Add(-5 * time.Hour).Unix()
	var tok5, tok7, sessions int64
	var limits map[string]limitInfo
	var limitAt int64
	var codex *codexRateLimits
	seen := map[string]bool{}
	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.ModTime().Before(cutoff) {
			return nil
		}
		fu := usageFiles[path]
		if fu == nil || fu.size != info.Size() || fu.modUnix != info.ModTime().Unix() {
			fu = parseUsageFile(path, parse, seen)
			fu.size, fu.modUnix = info.Size(), info.ModTime().Unix()
			usageFiles[path] = fu
		}
		active := false
		for _, p := range fu.points {
			if p.at < cutoff.Unix() {
				continue
			}
			active = true
			tok7 += p.tokens
			if p.at >= h5 {
				tok5 += p.tokens
			}
		}
		if active {
			sessions++
		}
		if fu.limits != nil && fu.limitAt > limitAt {
			limits, limitAt, codex = fu.limits, fu.limitAt, fu.codex
		}
		return nil
	})
	res := AgentUsage{UpdatedAt: now.UTC().Format(time.RFC3339), Sessions: sessions, codexLimits: codex, codexAt: limitAt}
	res.Windows = []UsageWindow{{Label: "5h", Tokens: tok5, Percent: -1}, {Label: "7d", Tokens: tok7, Percent: -1}}
	// 额度窗口按日志里的 window_minutes 对号入座；已过重置时间的百分比作废（窗口已清零）。
	for _, key := range []string{"primary", "secondary"} {
		l, ok := limits[key]
		if !ok || (l.resetsAt > 0 && l.resetsAt < now.Unix()) {
			continue
		}
		label := windowLabel(l.windowMin)
		resets := ""
		if l.resetsAt > 0 {
			resets = time.Unix(l.resetsAt, 0).UTC().Format(time.RFC3339)
		}
		matched := false
		for i := range res.Windows {
			if res.Windows[i].Label == label {
				res.Windows[i].Percent, res.Windows[i].ResetsAt, matched = l.percent, resets, true
			}
		}
		if !matched && label != "" {
			res.Windows = append(res.Windows, UsageWindow{Label: label, Tokens: -1, Percent: l.percent, ResetsAt: resets})
		}
	}
	return res, true
}

func parseUsageFile(path string, parse lineParser, seen map[string]bool) *fileUsage {
	fu := &fileUsage{}
	f, err := os.Open(path)
	if err != nil {
		return fu
	}
	defer f.Close()
	r := bufio.NewReaderSize(f, 256<<10)
	for {
		line, err := r.ReadBytes('\n')
		if len(line) > 0 && (bytes.Contains(line, []byte(`"usage"`)) || bytes.Contains(line, []byte(`"token_count"`))) {
			if p, lim, ok := parse(line, seen); ok {
				if p.tokens > 0 {
					fu.points = append(fu.points, p)
				}
				if lim != nil {
					fu.limits, fu.limitAt = lim, p.at
					if rl, ok := codexRateLimitsOf(line); ok {
						fu.codex = &rl
					}
				}
			}
		}
		if err == io.EOF || err != nil {
			break
		}
	}
	return fu
}

func parseTime(s string) int64 {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return 0
	}
	return t.Unix()
}

func parseClaudeLine(line []byte, seen map[string]bool) (usagePoint, map[string]limitInfo, bool) {
	var v struct {
		Type      string `json:"type"`
		Timestamp string `json:"timestamp"`
		RequestID string `json:"requestId"`
		Message   struct {
			ID    string `json:"id"`
			Usage struct {
				Input       int64 `json:"input_tokens"`
				Output      int64 `json:"output_tokens"`
				CacheCreate int64 `json:"cache_creation_input_tokens"`
			} `json:"usage"`
		} `json:"message"`
	}
	if json.Unmarshal(line, &v) != nil || v.Type != "assistant" {
		return usagePoint{}, nil, false
	}
	key := v.Message.ID + "|" + v.RequestID
	if key != "|" {
		if seen[key] {
			return usagePoint{}, nil, false
		}
		seen[key] = true
	}
	u := v.Message.Usage
	return usagePoint{at: parseTime(v.Timestamp), tokens: u.Input + u.Output + u.CacheCreate}, nil, true
}

func parseCodexLine(line []byte, _ map[string]bool) (usagePoint, map[string]limitInfo, bool) {
	type lim struct {
		UsedPercent     float64 `json:"used_percent"`
		WindowMinutes   int64   `json:"window_minutes"`
		ResetsInSeconds int64   `json:"resets_in_seconds"`
		ResetsAt        int64   `json:"resets_at"`
	}
	var v struct {
		Timestamp string `json:"timestamp"`
		Payload   struct {
			Type string `json:"type"`
			Info *struct {
				Last struct {
					Total int64 `json:"total_tokens"`
				} `json:"last_token_usage"`
			} `json:"info"`
			RateLimits *struct {
				Primary   *lim `json:"primary"`
				Secondary *lim `json:"secondary"`
			} `json:"rate_limits"`
		} `json:"payload"`
	}
	if json.Unmarshal(line, &v) != nil || v.Payload.Type != "token_count" {
		return usagePoint{}, nil, false
	}
	p := usagePoint{at: parseTime(v.Timestamp)}
	if v.Payload.Info != nil {
		p.tokens = v.Payload.Info.Last.Total
	}
	var limits map[string]limitInfo
	if rl := v.Payload.RateLimits; rl != nil {
		limits = map[string]limitInfo{}
		conv := func(l *lim) limitInfo {
			li := limitInfo{percent: l.UsedPercent, resetsAt: l.ResetsAt, windowMin: l.WindowMinutes}
			if li.resetsAt == 0 && l.ResetsInSeconds > 0 {
				li.resetsAt = p.at + l.ResetsInSeconds
			}
			return li
		}
		if rl.Primary != nil {
			limits["primary"] = conv(rl.Primary)
		}
		if rl.Secondary != nil {
			limits["secondary"] = conv(rl.Secondary)
		}
	}
	return p, limits, true
}
