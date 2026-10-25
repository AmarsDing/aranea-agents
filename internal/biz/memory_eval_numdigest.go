package biz

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// memory_eval_numdigest.go — AMC P3-c（G4 经验数据发现与模拟）数值序列
// digest 的纯函数层。
//
// 设计要点：
//   - 解析是「数值语法」不是「意图书面词表」：topic 取数字前最近的文字段
//     （末尾剥连接词、开头剥时间框架/代词），单位用字符类捕获（任意语言
//     单位都能存活），不维护单位/主题词枚举——与 P3 VIR 同一哲学。
//   - 日期形态进入前先遮蔽，日历数字永不进序列；手机号/身份证号/银行卡
//     这类 ≥8 位长数字段视为标识符照跳（PII），不污染度量序列。
//   - 摘要行是单行 canonical k=v 形式，既是 judge 可读的证据，也是下次
//     滚动更新的解析来源（round-trip 由单测钉死）。

// EvalNumericAtom is one (topic, value, unit) triple extracted from an atom.
type EvalNumericAtom struct {
	Topic string
	Value float64
	Unit  string
}

// evalNumValueRe: number with optional sign/decimal and an optional unit
// tail (≤4 chars of a language-agnostic class: letters/Han/%/°/℃//). Units
// must TOUCH the number ("3200步", "180lb", "128/86"→slash handled below) —
// a space-separated word after the number is deliberately left for the topic
// scanner ("my steps were 4500 steps" → topic "steps"), which keeps junk
// like "4500 today" out of the unit slot without any unit word list.
var evalNumValueRe = regexp.MustCompile(`(-?\d+(?:\.\d+)?)([\p{Han}A-Za-z%/℃°]{0,4})`)

// evalNumWordRun matches one CJK/latin letter run immediately before a
// (possibly connector-prefixed) number on the scan path.
// evalNumWordRun matches one CJK/latin letter run (helper for doc purposes;
// the scanner below uses isEvalNumWordRune directly).
var evalNumWordRe = regexp.MustCompile(`[\p{Han}A-Za-z]+`)

// evalNumDateMask replaces absolute date forms with spaces BEFORE scanning,
// so 2025年5月6日 / 2025-05-06 / May 1 / in 2025 never seed a metric series.
// Latin-month and bare-year forms are calendar grammar, same as the CN/ISO
// forms — not intent wordlists.
var evalNumDateMask = []*regexp.Regexp{
	regexp.MustCompile(`\d{4}年\d{1,2}月\d{1,2}日?`),
	regexp.MustCompile(`\b\d{4}-\d{1,2}-\d{1,2}\b`),
	regexp.MustCompile(`\b\d{1,2}/\d{1,2}/\d{4}\b`),
	regexp.MustCompile(`\b\d{1,2}月\d{1,2}日?`),
	regexp.MustCompile(`(?i)\b(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\.?\s+\d{1,2}(?:st|nd|rd|th)?\b`),
	regexp.MustCompile(`(?i)\b\d{1,2}(?:st|nd|rd|th)?\s+(?:of\s+)?(?:jan|feb|mar|apr|may|jun|jul|aug|sep|sept|oct|nov|dec)[a-z]*\b`),
	regexp.MustCompile(`(?i)\b(?:in|on|of|year)\s+(?:19|20)\d{2}\b`),
	regexp.MustCompile(`(?:19|20)\d{2}年`),
}

// evalNumIdentMask replaces identifier-tagged digit runs BEFORE scanning
// (AMC P3 评审 C4): 工号/订单号/尾号/id: 这类「标签词+数字」是标识符语法——
// 与日历数字同理，永远不是度量序列。标签词≥3 位数字才收，裸标签/个位数序
// 号（"no.1 优先"）不动。短数字段（9527/3688）正是 ≥8 位纯数字守卫管不到
// 的盲区。
// 评审 round-3（F2/F3/F4）统一为单条跨语正则：标签表与连接词表不分中英
// （中文标签也常见 "工号-9527"/"工号is9527"，CN 用户说 ID 的常态说法是
// "我的id是9527"，过去时 "my id was 3688" 同样是标识符陈述）；连接词覆盖
// 是/为/is/was/were/:/#/- 全组合。
var evalNumIdentMask = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:工号|编号|员工号|学号|考号|会员号|订单号|卡号|账号|账户号|尾号|票号|单号|房号|门牌号|护照号|证件号|病历号|\b(?:id|uid|ssn|code)\b)\s*(?:是|为|is|was|were|[:：#-])?\s*\d{3,}`),
}

// evalNumUnitParticles are aspect/mood close-particles (了/的/是/呢…) that
// the language-agnostic unit capture may swallow when they hug the number
// ("3200步了" → raw unit "步了"). They are grammar, never measure words —
// trimmed from both unit edges. Generic counters (个/次/条) stay: dropping
// them risks eating real measure words.
var evalNumUnitParticles = []rune{'了', '的', '是', '在', '也', '都', '还', '又', '和', '与', '或', '呢', '吧', '啊', '啦', '嘛'}

// evalNumTopicPrefixFillers are time-frame / pronoun / article lead-ins
// stripped from the LEFT edge of a topic run ("本周步数"→"步数",
// "我走了"→"走了"). Bare latin pronouns/articles strip whole words.
var evalNumTopicPrefixFillers = []string{
	"本周", "这周", "上周", "下周", "本日", "前天", "后天",
	"今天", "今日", "明天", "昨日", "昨天", "早上", "早晨", "上午", "中午", "下午", "晚上", "凌晨",
	"刚刚", "刚才", "最近", "当天", "当时",
	"我", "你", "他", "她", "它", "咱", "的",
	"my", "the", "our", "your", "his", "her", "their", "this", "that", "a", "an",
}

// evalNumTopicSuffixConnectors are grammar links / light verbs stripped from
// the RIGHT edge of a topic run ("步数分别是"→"步数", "steps were"→"steps").
// Kept to grammatical copulas / connectors / measurement verbs, not domain
// vocabulary. A run that strips empty is skipped by the scanner (it keeps
// walking left), which is how "weight dropped to 180" still lands on
// "weight".
var evalNumTopicSuffixConnectors = []string{
	"分别是", "分别", "主要是", "大概是", "大约是",
	"测得是", "测得", "测了", "记录是", "记录为", "记录的", "量的是", "量了", "量到",
	"达到了", "达到", "涨到", "涨到是", "升到", "降到", "跌到",
	"是", "的", "了", "到", "为", "约", "有",
	"dropped to", "rose to", "went to", "down to", "up to",
	"dropped", "rose", "went", "down", "up", "reached", "recorded", "logged",
	"was", "were", "is", "are", "at", "of", "to",
}

// evalNumSeriesGap is the max raw-string distance between a value and the
// next bare value that still INHERITS the previous topic (…是3200、4500步).
const evalNumSeriesGap = 4

// ParseEvalNumericAtoms extracts (topic, value, unit) triples from one
// statement atom. Role prefixes are stripped the same way as elsewhere.
// Hypothetical statements (如果…就/if…would) yield no triples — a
// counterfactual number must never seed a metric series (AMC P3 评审 C6).
// 评审 round-3 F6：混合句按从句剥离（「我今天走了8000步，如果状态好明天就
// 走10000步」→ 只收 8000 步），真实观测不再被整句假设守卫误丢。
func ParseEvalNumericAtoms(stmt string) []EvalNumericAtom {
	s := StripEvalHypotheticalClauses(stripRolePrefix(stmt))
	if s == "" {
		return nil
	}
	masked := s
	for _, re := range evalNumDateMask {
		masked = re.ReplaceAllString(masked, " ")
	}
	for _, re := range evalNumIdentMask {
		masked = re.ReplaceAllString(masked, " ")
	}
	var atoms []EvalNumericAtom
	matches := evalNumValueRe.FindAllStringSubmatchIndex(masked, -1)
	lastTopic := ""
	lastNumEnd := -1
	for _, m := range matches {
		numStart, numEnd := m[2], m[3]
		unitStr := masked[m[4]:m[5]]
		// Identifier guard: ≥8 digit runs are phones/IDs/cards, not metrics.
		digits := strings.TrimPrefix(masked[numStart:numEnd], "-")
		if !strings.Contains(digits, ".") && len(digits) >= 8 {
			continue
		}
		// Blood-pressure style "128/86": skip the second half of a pair, and
		// do not let the numerator's unit capture eat the slash itself.
		if numStart >= 2 && masked[numStart-1] == '/' {
			if unicode.IsDigit(runeAt(masked, numStart-2)) {
				continue
			}
		}
		if unitStr == "/" && m[5] < len(masked) && masked[m[5]] >= '0' && masked[m[5]] <= '9' {
			unitStr = ""
		}
		// Find the nearest usable topic run left of the number: scan word
		// runs right→left (bounded window, rune-correct); a run that strips
		// to all-filler ("were", "dropped to") does not qualify — keep
		// walking.
		topic := ""
		const lookback = 24
		pos := numStart - 1
		for pos >= 0 && numStart-pos <= lookback && topic == "" {
			r, sz := utf8.DecodeLastRuneInString(masked[:pos+1])
			if unicode.IsDigit(r) {
				break
			}
			if !isEvalNumWordRune(r) {
				pos -= sz
				continue
			}
			end := pos
			pos -= sz
			for pos >= 0 {
				r2, sz2 := utf8.DecodeLastRuneInString(masked[:pos+1])
				if !isEvalNumWordRune(r2) {
					break
				}
				pos -= sz2
			}
			topic = normalizeEvalNumTopic(masked[pos+1 : end+1])
		}
		if topic == "" {
			if lastTopic != "" && numStart-lastNumEnd <= evalNumSeriesGap {
				topic = lastTopic
			} else {
				continue
			}
		}
		val, err := strconv.ParseFloat(masked[numStart:numEnd], 64)
		if err != nil {
			continue
		}
		atoms = append(atoms, EvalNumericAtom{Topic: topic, Value: val, Unit: trimEvalNumUnit(unitStr)})
		lastTopic = topic
		lastNumEnd = numEnd
	}
	return atoms
}

// trimEvalNumUnit peels aspect/mood particles the unit class swallowed from
// both edges ("步了"→"步", "了步"→"步"); an all-particle unit becomes "".
func trimEvalNumUnit(u string) string {
	isParticle := func(r rune) bool {
		for _, p := range evalNumUnitParticles {
			if r == p {
				return true
			}
		}
		return false
	}
	runes := []rune(u)
	for len(runes) > 0 && isParticle(runes[len(runes)-1]) {
		runes = runes[:len(runes)-1]
	}
	for len(runes) > 0 && isParticle(runes[0]) {
		runes = runes[1:]
	}
	return string(runes)
}

func isEvalNumWordRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.IsLetter(r)
}

func runeAt(s string, byteIdx int) rune {
	if byteIdx < 0 || byteIdx >= len(s) {
		return 0
	}
	r, _ := utf8.DecodeRuneInString(s[byteIdx:])
	return r
}

// normalizeEvalNumTopic strips grammar fillers from both edges of a topic
// run and bounds its length (≤6 Han chars / ≤12 latin chars from the right).
func normalizeEvalNumTopic(topic string) string {
	t := strings.ToLower(strings.TrimSpace(topic))
	for iter := 0; iter < 3 && t != ""; iter++ {
		changed := false
		for _, f := range evalNumTopicPrefixFillers {
			if strings.HasPrefix(t, f) {
				t = strings.TrimSpace(strings.TrimPrefix(t, f))
				changed = true
			}
		}
		for _, c := range evalNumTopicSuffixConnectors {
			if strings.HasSuffix(t, c) {
				t = strings.TrimSpace(strings.TrimSuffix(t, c))
				changed = true
			}
		}
		if !changed {
			break
		}
	}
	// Bound from the right: last ≤6 Han chars or last ≤12 latin letters.
	runes := []rune(t)
	if len(runes) == 0 {
		return ""
	}
	if unicode.Is(unicode.Han, runes[len(runes)-1]) {
		if len(runes) > 6 {
			runes = runes[len(runes)-6:]
		}
	} else {
		if len(runes) > 12 {
			runes = runes[len(runes)-12:]
		}
	}
	return string(runes)
}

// SeriesKey canonicalizes a triple's series identity: the normalized TOPIC is
// the series. Unit-first keying is NOT used — it would merge different
// metrics that merely share a unit ("体重75公斤" vs "背包5公斤" → a garbage
// "公斤" series); same metric under 步数/走…步 phrasings splits into two
// benign series instead, which a trend query surfaces together. Unit stays
// display metadata in the rendered digest line.
func (a EvalNumericAtom) SeriesKey() string {
	if t := strings.TrimSpace(a.Topic); t != "" {
		return t
	}
	return strings.ToLower(strings.TrimSpace(a.Unit))
}

// EvalNumSeries is the rolling digest state for one series key.
type EvalNumSeries struct {
	Topic      string
	Count      int
	Sum        float64
	Min, Max   float64
	Last       float64
	LastUnit   string
	FirstValue float64
	FirstUnit  string
	FirstDay   string
	LastDay    string
}

// Add folds one observation into the series.
func (s *EvalNumSeries) Add(value float64, unit, day string) {
	if s.Count == 0 {
		s.Min, s.Max = value, value
		s.FirstValue = value
		s.FirstUnit = unit
		s.FirstDay = day
	} else {
		if value < s.Min {
			s.Min = value
		}
		if value > s.Max {
			s.Max = value
		}
	}
	s.Count++
	s.Sum += value
	s.Last = value
	if unit != "" {
		s.LastUnit = unit
	} else if s.LastUnit == "" {
		s.LastUnit = s.FirstUnit
	}
	if day != "" {
		if s.FirstDay == "" {
			s.FirstDay = day
		}
		if day < s.FirstDay {
			s.FirstDay = day
		}
		if s.LastDay == "" || day > s.LastDay {
			s.LastDay = day
		}
	}
}

// Mean returns the running mean.
func (s EvalNumSeries) Mean() float64 {
	if s.Count == 0 {
		return 0
	}
	return s.Sum / float64(s.Count)
}

// TrendText renders a simple linear slope extrapolation over the first→last
// span ("上升约+3.2%/天" / "下降约-5.1%/day"), or "" for single-point/undated
// series — the digest line omits the trend segment then.
func (s EvalNumSeries) TrendText() string {
	if s.Count < 2 || s.FirstDay == "" || s.LastDay == "" || s.FirstDay == s.LastDay {
		return ""
	}
	d1, err1 := time.Parse("2006-01-02", s.FirstDay)
	d2, err2 := time.Parse("2006-01-02", s.LastDay)
	if err1 != nil || err2 != nil {
		return ""
	}
	days := d2.Sub(d1).Hours() / 24
	if days <= 0 {
		return ""
	}
	slope := (s.Last - s.FirstValue) / days
	mean := s.Mean()
	if mean == 0 {
		return ""
	}
	pct := slope / math64Abs(mean) * 100
	dir := "上升"
	if pct < -0.5 {
		dir = "下降"
	} else if pct >= -0.5 && pct <= 0.5 {
		dir = "平稳"
	}
	return fmt.Sprintf("%s约%+.1f%%/day", dir, pct)
}

func math64Abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// RenderEvalSeriesDigest renders the canonical single-line digest. Format
// (round-tripped by ParseEvalSeriesDigest):
// [series-digest: <topic>] count=N last=V<unit> mean=V<unit> min=V<unit> max=V<unit> firstValue=V<unit> firstDay=D lastDay=D sum=S[ trend=T]
// sum carries the exact running sum (shortest round-trip form): mean stays
// %.1f for judge readability, and re-deriving Sum from it would accumulate
// drift over rotations (AMC P3 评审 C3). Older rows without sum fall back to
// mean*count in the parser.
func (s EvalNumSeries) RenderEvalSeriesDigest() string {
	b := &strings.Builder{}
	fmt.Fprintf(b, "[series-digest: %s] count=%d last=%s%s mean=%.1f%s min=%s%s max=%s%s firstValue=%s%s firstDay=%s lastDay=%s sum=%s",
		s.Topic, s.Count,
		formatEvalNumValue(s.Last), s.LastUnit,
		s.Mean(), s.LastUnit,
		formatEvalNumValue(s.Min), s.LastUnit,
		formatEvalNumValue(s.Max), s.LastUnit,
		formatEvalNumValue(s.FirstValue), s.FirstUnit,
		s.FirstDay, s.LastDay,
		formatEvalNumValue(s.Sum),
	)
	if tr := s.TrendText(); tr != "" {
		fmt.Fprintf(b, " trend=%s", tr)
	}
	return b.String()
}

func formatEvalNumValue(v float64) string {
	return strconv.FormatFloat(v, 'g', -1, 64)
}

// evalSeriesDigestRe parses the rendered digest line back into counters. The
// trailing sum group is OPTIONAL so rows rendered by older versions still
// parse (fallback = mean*count). ALL value fields accept exponent notation —
// formatEvalNumValue('g') switches to %e for extreme magnitudes (AMC P3 评审
// round-3 F1：只放开 sum 会让 1e-05 这类值把 last/min/max 静默解析成 1)。
// 值类用严格数值文法（指数段必须带数字）：单位以 e/E 起头（eV/em）时
// 贪吃的宽松类会把单位首字母吞进数值使 ParseFloat 归零——strict 文法下
// 「5eV」解析为 5+eV（round-3b）。
var evalSeriesDigestRe = regexp.MustCompile(`^\[series-digest: ([^\]]+)\] count=(\d+) last=(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)([^\s]*) mean=(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)([^\s]*) min=(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)([^\s]*) max=(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)([^\s]*) firstValue=(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?)([^\s]*) firstDay=(\d{4}-\d{2}-\d{2}) lastDay=(\d{4}-\d{2}-\d{2})(?: sum=(-?\d+(?:\.\d+)?(?:[eE][+-]?\d+)?))?(?: trend=(\S+))?$`)

// ParseEvalSeriesDigest round-trips a rendered digest row's statement back
// into series state (the write path re-parses the previous digest row to roll
// counters forward). Non-digest statements report ok=false.
func ParseEvalSeriesDigest(stmt string) (EvalNumSeries, bool) {
	m := evalSeriesDigestRe.FindStringSubmatch(strings.TrimSpace(stmt))
	if m == nil {
		return EvalNumSeries{}, false
	}
	f := func(x string) float64 {
		v, _ := strconv.ParseFloat(x, 64)
		return v
	}
	n, _ := strconv.Atoi(m[2])
	sum := f(m[15])
	if m[15] == "" {
		sum = f(m[5]) * float64(n) // 老行无 sum 字段：mean×count 回退。
	}
	return EvalNumSeries{
		Topic:      m[1],
		Count:      n,
		Sum:        sum,
		Last:       f(m[3]),
		LastUnit:   m[4],
		Min:        f(m[7]),
		Max:        f(m[9]),
		FirstValue: f(m[11]),
		FirstUnit:  m[12],
		FirstDay:   m[13],
		LastDay:    m[14],
	}, true
}

// IsEvalSeriesDigestStatement reports whether a statement is a canonical
// digest row (AMC P3 评审 C5: full canonical match, NOT a bare prefix — a
// role-less user statement merely starting with "[series-digest: " must not
// be adopted or rotated as derived state).
func IsEvalSeriesDigestStatement(stmt string) bool {
	return evalSeriesDigestRe.MatchString(strings.TrimSpace(stmt))
}

// IsEvalSeriesDigestPinworthy reports whether a digest row should be pinned
// for trend queries / floor-exempt (AMC P3 评审 C4): only a real SERIES
// (count≥2) earns pinning — a single-observation digest restates its atom and
// is pure noise at the top of the evidence list.
func IsEvalSeriesDigestPinworthy(stmt string) bool {
	s, ok := ParseEvalSeriesDigest(stmt)
	return ok && s.Count >= 2
}
