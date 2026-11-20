package orggraph

// 总经理 Agent 的巡视与汇报规则（设计稿 03-看板 §14.3）。
//
// 巡视只记事实（不调模型）；汇报先用这里的规则整理事实与结论草稿，再交给
// 总经理用自己的模型写成「结论 + 理由」，模型没答出来时回落到这里的规则文本。

import (
	"fmt"
	"strings"
	"time"
)

const (
	maxFindings = 10
	maxReasons  = 5
	// 做完等验收超过这么久，算一条要看一眼的事。
	reviewOverdue = 4 * time.Hour
	// 截止在这么久以内的待拍板，算快到期。
	dueSoon = 2 * time.Hour
)

// Finding 是巡视发现的一件事。
type Finding struct {
	Level string `json:"level"` // attention | info
	Text  string `json:"text"`
	Link  string `json:"link,omitempty"`
}

// Patrol 是一次巡视的结果（存进定时任务运行记录的 output_json.gm）。
type Patrol struct {
	CompanyID   string    `json:"company_id"`
	Company     string    `json:"company"`
	State       string    `json:"state"`
	Loop        Loop      `json:"loop"`
	AgentCount  int       `json:"agent_count"`
	ActiveToday int       `json:"active_today"`
	Failed      int       `json:"failed"`
	Calls       int64     `json:"calls"`
	CostMicro   int64     `json:"cost_micro"`
	Findings    []Finding `json:"findings"`
}

// PatrolCompany 按规则巡视一家公司：卡住的交办 → 失败的 Agent → 高风险或快到期的
// 待拍板 → 久未验收 → 今天没动静的部门，最多 maxFindings 条。
func PatrolCompany(c CompanyNode, now time.Time) Patrol {
	p := Patrol{
		CompanyID:   c.ID,
		Company:     c.Name,
		State:       c.State,
		Loop:        c.Loop,
		AgentCount:  c.AgentCount,
		ActiveToday: c.ActiveToday,
		Failed:      c.Failed,
		Calls:       c.Calls,
		CostMicro:   c.CostMicro,
		Findings:    []Finding{},
	}
	names := map[string]string{}
	var members []BoardAgent
	if c.CEO != nil {
		members = append(members, *c.CEO)
	}
	for _, d := range c.Depts {
		members = append(members, d.Agents...)
	}
	for _, a := range members {
		names[a.ID] = a.Name
	}
	add := func(level, text, link string) {
		if len(p.Findings) < maxFindings {
			p.Findings = append(p.Findings, Finding{Level: level, Text: text, Link: link})
		}
	}

	for _, w := range c.WorkItems {
		if w.State == WorkStuck {
			add(StateAttention, fmt.Sprintf("交办「%s」给%s，卡住了%s", w.Title, nameOr(names[w.AssigneeID], "Agent"), since(w.At, now)), w.To)
		}
	}
	seen := map[string]bool{}
	for _, a := range members {
		if a.Failed && !seen[a.ID] {
			seen[a.ID] = true
			add(StateAttention, fmt.Sprintf("%s 最近一次运行失败%s", a.Name, paren(a.LastRunAt, now)), "")
		}
	}
	plain := 0
	for _, item := range c.WaitingItems {
		due := ParseTime(item.DueAt)
		switch {
		case item.RiskLevel == "high":
			add(StateAttention, fmt.Sprintf("等你拍板（高风险）：%s", item.Title), item.Link)
		case !due.IsZero() && due.Sub(now) <= dueSoon:
			add(StateAttention, fmt.Sprintf("等你拍板（%s 到期）：%s", clock(item.DueAt, now), item.Title), item.Link)
		default:
			plain++
		}
	}
	if plain > 0 {
		add(StateAttention, fmt.Sprintf("还有 %d 件等你拍板", plain), "/today")
	}
	for _, w := range c.WorkItems {
		t := ParseTime(w.At)
		if w.State == WorkReview && !t.IsZero() && now.Sub(t) >= reviewOverdue {
			add(StateAttention, fmt.Sprintf("「%s」做完了，等你验收 %d 小时", w.Title, int(now.Sub(t).Hours())), w.To)
		}
	}
	if c.ActiveToday > 0 {
		for _, d := range c.Depts {
			if d.AgentCount > 0 && d.ActiveToday == 0 && d.Loop.Running == 0 {
				add("info", fmt.Sprintf("%s今天没动静（%d 个在岗）", d.Name, d.AgentCount), "")
			}
		}
	}
	return p
}

// Conclusion 是汇报的结论草稿：按规则取第一条成立的。
func Conclusion(p Patrol) string {
	var parts []string
	for _, kv := range []struct {
		n    int
		unit string
	}{{p.Loop.Waiting, "等你拍板 %d 件"}, {p.Loop.Stuck, "卡住 %d 件"}, {p.Loop.Review, "待验收 %d 件"}, {p.Failed, "失败 %d 个"}} {
		if kv.n > 0 {
			parts = append(parts, fmt.Sprintf(kv.unit, kv.n))
		}
	}
	switch {
	case len(parts) > 0:
		return "要看一眼：" + strings.Join(parts, "、")
	case p.ActiveToday > 0 || p.Loop.Running > 0:
		return fmt.Sprintf("正常：今天 %d/%d 跑过，%d 件在做，花费 %s", p.ActiveToday, p.AgentCount, p.Loop.Running, Money(p.CostMicro))
	case p.AgentCount > 0:
		return fmt.Sprintf("今天没动静：%d 个 Agent 在岗，今天都没跑过", p.AgentCount)
	default:
		return "没人在岗"
	}
}

// Reasons 是汇报的理由草稿：最严重的发现在前，最后一条是和上次汇报比的变化。
// prev 为空表示没有上次汇报。
func Reasons(p Patrol, prev *Patrol, prevAt string, now time.Time) []string {
	out := []string{}
	for _, f := range p.Findings {
		if len(out) >= maxReasons-1 {
			break
		}
		out = append(out, f.Text)
	}
	if len(out) == 0 {
		out = append(out, fmt.Sprintf("今天 %d/%d 跑过，%d 件在做，调用 %d 次，花费 %s", p.ActiveToday, p.AgentCount, p.Loop.Running, p.Calls, Money(p.CostMicro)))
	}
	if prev != nil {
		if d := delta(*prev, p); d != "" {
			out = append(out, fmt.Sprintf("和上次汇报（%s）比：%s", clock(prevAt, now), d))
		}
	}
	return out
}

func delta(a, b Patrol) string {
	var parts []string
	for _, kv := range []struct {
		name     string
		from, to int
	}{{"等你拍板", a.Loop.Waiting, b.Loop.Waiting}, {"在做", a.Loop.Running, b.Loop.Running}, {"卡住", a.Loop.Stuck, b.Loop.Stuck}, {"待验收", a.Loop.Review, b.Loop.Review}} {
		if kv.from != kv.to {
			parts = append(parts, fmt.Sprintf("%s %d → %d", kv.name, kv.from, kv.to))
		}
	}
	if len(parts) == 0 {
		return "四格没有变化"
	}
	return strings.Join(parts, "，")
}

var stateLabel = map[string]string{StateAttention: "要看一眼", StateOK: "正常", StateIdle: "今天没动静", StateEmpty: "没人在岗"}

// ReportPrompt 是发给总经理的定时汇报消息：系统统计的事实 + 结论 / 理由草稿 + 回答格式。
func ReportPrompt(p Patrol, conclusion string, reasons []string, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "【定时汇报 · %s · %s】\n", p.Company, now.Format("15:04"))
	fmt.Fprintf(&b, "你是%s的总经理。下面是系统刚统计的本公司情况，请据此给老板写一条汇报。\n\n", p.Company)
	b.WriteString("系统统计：\n")
	fmt.Fprintf(&b, "- 状态：%s\n", nameOr(stateLabel[p.State], p.State))
	fmt.Fprintf(&b, "- 等你拍板 %d 件，在做 %d 件，卡住 %d 件，待验收 %d 件\n", p.Loop.Waiting, p.Loop.Running, p.Loop.Stuck, p.Loop.Review)
	fmt.Fprintf(&b, "- 今天跑过 / 在岗：%d/%d，今日调用 %d 次，花费 %s\n", p.ActiveToday, p.AgentCount, p.Calls, Money(p.CostMicro))
	if len(p.Findings) > 0 {
		b.WriteString("- 需要注意：\n")
		for i, f := range p.Findings {
			fmt.Fprintf(&b, "  %d. %s\n", i+1, f.Text)
		}
	}
	fmt.Fprintf(&b, "\n系统的结论草稿：%s\n理由草稿：\n", conclusion)
	for _, r := range reasons {
		fmt.Fprintf(&b, "- %s\n", r)
	}
	b.WriteString("\n要求：\n1. 只用上面的事实，不编造数字或事项；可以改写草稿，也可以照用。\n2. 不派活，不调用工具。\n3. 严格按下面的格式回答，不要写别的：\n结论：<一句话>\n理由：\n- <最多 5 条，每条一句>\n")
	return b.String()
}

// ParseReport 从总经理的回答里取出结论与理由；取不到结论时 ok=false。
func ParseReport(reply string) (conclusion string, reasons []string, ok bool) {
	inReasons := false
	for _, raw := range strings.Split(reply, "\n") {
		line := strings.TrimSpace(strings.ReplaceAll(raw, "**", ""))
		line = strings.TrimSpace(strings.TrimLeft(line, "#> "))
		if line == "" {
			continue
		}
		if rest, found := cutLabel(line, "结论"); found {
			if rest != "" && conclusion == "" {
				conclusion = rest
			}
			inReasons = false
			continue
		}
		if rest, found := cutLabel(line, "理由"); found {
			inReasons = true
			if rest != "" {
				reasons = append(reasons, rest)
			}
			continue
		}
		if !inReasons || len(reasons) >= maxReasons {
			continue
		}
		if item := bullet(line); item != "" {
			reasons = append(reasons, item)
		}
	}
	return conclusion, reasons, conclusion != ""
}

// cutLabel 识别「结论：…」「结论: …」。
func cutLabel(line, label string) (string, bool) {
	if !strings.HasPrefix(line, label) {
		return "", false
	}
	rest := strings.TrimPrefix(line, label)
	for _, sep := range []string{"：", ":"} {
		if strings.HasPrefix(rest, sep) {
			return strings.TrimSpace(strings.TrimPrefix(rest, sep)), true
		}
	}
	return "", false
}

// bullet 去掉列表符号（- • · * 或 1. 1、）；不是列表行返回空。
func bullet(line string) string {
	for _, mark := range []string{"- ", "-", "• ", "•", "· ", "·", "* "} {
		if strings.HasPrefix(line, mark) {
			return strings.TrimSpace(strings.TrimPrefix(line, mark))
		}
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i > 0 && i < len(line) {
		rest := line[i:]
		for _, sep := range []string{".", "、", ")", "）"} {
			if strings.HasPrefix(rest, sep) {
				return strings.TrimSpace(strings.TrimPrefix(rest, sep))
			}
		}
	}
	return ""
}

// Money 把微美元写成 $0.52；0 写 $0。
func Money(micro int64) string {
	if micro <= 0 {
		return "$0"
	}
	if micro < 10_000 {
		return fmt.Sprintf("$%.4f", float64(micro)/1e6)
	}
	return fmt.Sprintf("$%.2f", float64(micro)/1e6)
}

func nameOr(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

// clock 写成 HH:mm；不是今天带上月-日。
func clock(iso string, now time.Time) string {
	t := ParseTime(iso)
	if t.IsZero() {
		return ""
	}
	t = t.In(now.Location())
	if SameDay(iso, now) {
		return t.Format("15:04")
	}
	return t.Format("01-02 15:04")
}

func since(iso string, now time.Time) string {
	if c := clock(iso, now); c != "" {
		return "（从 " + c + " 起）"
	}
	return ""
}

func paren(iso string, now time.Time) string {
	if c := clock(iso, now); c != "" {
		return "（" + c + "）"
	}
	return ""
}
