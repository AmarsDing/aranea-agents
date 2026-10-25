package biz

import (
	"regexp"
	"strings"
	"unicode"
)

var (
	likeStatementRe     = regexp.MustCompile(`(?i)\b(?:i\s+)?(?:like|love|prefer|hate|enjoy|dislike)s?\s+`)
	favoriteStatementRe = regexp.MustCompile(`(?i)\b(?:my\s+)?favou?rite\s+\w+\s+(?:is|was|are)\s+`)
	liveStatementRe     = regexp.MustCompile(`(?i)\b(?:i\s+)?(?:live|lives|lived)\s+(?:in|at)\s+`)
	nameStatementRe     = regexp.MustCompile(`(?i)\b(?:my name is|i am called)\s+`)
	chineseFavoriteRe   = regexp.MustCompile(`最喜欢的([^是为改换变，。,\s]{1,12})(?:是|为|改成|换成|改为|变成|变为)`)
)

// CanonicalizeFactKind maps extractor / tool kinds onto the remember-tool
// vocabulary so identity facts are not split across preference / profile /
// user_identity / domain_knowledge. Statement heuristics override a wrong
// LLM label (e.g. 工号 tagged as preference) and fill vague kinds
// (general/fact/event) from the statement itself.
func CanonicalizeFactKind(kind, statement string) string {
	if LooksLikeUserIdentityStatement(statement) {
		return "user_identity"
	}
	// AMC P3 / E3：健康/家庭长期状态升入 user_identity（evergreen），
	// 挂在名牌身份之后、LLM 标签映射之前——statement 启发式优先于错标。
	if LooksLikeHealthIdentityStatement(statement) {
		return "user_identity"
	}
	mapped := mapFactKindToken(kind)
	if vagueFactKind(mapped) {
		if LooksLikePreferenceStatement(statement) {
			return "preference"
		}
		if LooksLikeConstraintStatement(statement) {
			return "constraint"
		}
	}
	return mapped
}

func mapFactKindToken(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "identity", "user_identity":
		return "user_identity"
	case "preference", "user_preference", "profile":
		return "preference"
	case "constraint":
		return "constraint"
	case "instruction", "agent_instruction":
		return "agent_instruction"
	case "domain_knowledge":
		return "domain_knowledge"
	default:
		k := strings.ToLower(strings.TrimSpace(kind))
		if k == "" {
			return "general"
		}
		return k
	}
}

func vagueFactKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "general", "fact", "event", "knowledge":
		return true
	default:
		return false
	}
}

// LooksLikeUserIdentityStatement reports stable user-identity statements
// (employee id, name, role) that must not be stored as preference.
func LooksLikeUserIdentityStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	for _, p := range []string{"工号", "我叫", "负责", "employee id", "employee number", "my name is", "i am called"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return nameStatementRe.MatchString(s)
}

// LooksLikeHealthIdentityStatement reports durable health/family-status
// statements (AMC P3 / E3 背景健康关怀). 「我患有糖尿病/我对青霉素过敏/
// 我离异独居」这类事实是用户长期状态而非一次性事件：浅分类为 event 会
// 随时间衰减、在关怀类提问时权重沉底。归入 user_identity 后与身份事实
// 同列（evergreen 不衰减、importance 随 kind 同档）。
// 词表只收状态谓词（患有/确诊/在服用/过敏/丧偶/离异/独居/怀孕），不收
// 病情名词本身——「今天胃疼」是事件不是状态。代价接受：个别误伤句获得
// evergreen，代价远小于健康事实沉底。
func LooksLikeHealthIdentityStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	for _, p := range []string{
		"患有", "确诊", "长期服用", "长期服药", "在服用", "在服药",
		"过敏", "丧偶", "丧亲", "去世", "过世", "离异", "独居",
		"独自居住", "独自生活", "怀孕", "哺乳期", "慢阻肺", "哮喘史",
		"残疾", "抑郁症", "焦虑症", "长期失眠", "失眠症",
		"diabetic", "diabetes", "asthma", "allergic", "allergy",
		"on medication", "widowed", "divorced", "live alone",
		"lives alone", "living alone", "pregnant", "chronic", "disabled",
		"passed away", "insomnia",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	// AMC P3 评审 C7：动词短语必须带限定宾语——裸 "lost my " 会误伤
	// "lost my keys"，裸 "i take " 会误伤 "i take the bus"。宾语窗口取
	// 动词后最多 3 个词（"lost my elderly father"/"blood pressure
	// medication"），命中 kin/med 词才算长期状态。
	if anyHealthWord(healthObjectWords(s, "lost my "), healthKinWords) {
		return true
	}
	for _, m := range []string{"i take ", "i'm taking"} {
		if anyHealthWord(healthObjectWords(s, m), healthMedWords) {
			return true
		}
	}
	if strings.Contains(s, "i have diabetes") || strings.Contains(s, "i have asthma") {
		return true
	}
	return false
}

// healthKinWords: kin nouns making "lost my <kin>" a durable family status.
// 单复数并收（parents 在列后 children/sons/… 缺位 asymmetric——AMC P3 评审
// R2-C7-2：复数亲属丧亲同样是长期家庭状态）。
var healthKinWords = map[string]bool{
	"father": true, "mother": true, "parent": true, "parents": true,
	"husband": true, "wife": true, "spouse": true, "spouses": true,
	"partner": true, "partners": true,
	"son": true, "sons": true, "daughter": true, "daughters": true,
	"child": true, "children": true, "kid": true, "kids": true,
	"brother": true, "sister": true, "sibling": true, "siblings": true,
	"grandfather": true, "grandmother": true, "grandparent": true, "grandparents": true,
	"uncle": true, "aunt": true,
}

// healthMedWords: medication lexemes making "i take/taking <med>" a durable
// health status (generic pill/dose words + common drug names + CN lexemes).
var healthMedWords = map[string]bool{
	"insulin": true, "aspirin": true, "metformin": true, "statin": true, "statins": true,
	"pill": true, "pills": true, "tablet": true, "tablets": true,
	"medication": true, "medications": true, "medicine": true, "medicines": true,
	"dose": true, "doses": true, "dosage": true, "mg": true,
	"vitamin": true, "vitamins": true, "supplement": true, "supplements": true,
	"antidepressant": true, "antidepressants": true, "antibiotic": true, "antibiotics": true,
	"药": true, "维生素": true, "胰岛素": true, "降压药": true, "降糖药": true,
}

// anyHealthWord reports whether any of the candidate object words is in set.
func anyHealthWord(ws []string, set map[string]bool) bool {
	for _, w := range ws {
		if set[w] {
			return true
		}
	}
	return false
}

// healthObjectWords returns the first up to 3 words (latin letters or Han
// runs) following marker inside lowercased s — the object candidates of a
// health verb phrase. nil when the marker is absent or ends the string.
// AMC P3 评审 R2-C7-1：Han↔latin 字类切换即分词（维生素c → 维生素/c），
// 混写词条才能命中 CN 词表。
func healthObjectWords(s, marker string) []string {
	idx := strings.Index(s, marker)
	if idx < 0 {
		return nil
	}
	rest := strings.TrimLeftFunc(s[idx+len(marker):], unicode.IsSpace)
	var out []string
	var cur []rune
	curClass := 0 // 0 = 空 · 1 = Han · 2 = latin
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = nil
		}
		curClass = 0
	}
	for _, r := range rest {
		class := 0
		switch {
		case unicode.Is(unicode.Han, r):
			class = 1
		case r >= 'a' && r <= 'z':
			class = 2
		}
		if class == 0 {
			flush()
			if len(out) >= 3 {
				return out
			}
			continue
		}
		if curClass != 0 && class != curClass {
			flush()
		}
		curClass = class
		cur = append(cur, r)
	}
	flush()
	return out
}

// LooksLikeAbsenceMetaStatement reports statements whose content is "the user
// asked X but the system has no record of it" — a conversation meta-observation,
// not a durable fact. Persisting them poisons recall: the absence statement
// outranks the true fact on recency, the model parrots "not found", and the
// reply is saved as yet another absence statement (2026-08-26 domain-B
// regression pollution loop). Genuine negative facts ("原号码已作废") do not
// carry the inquiry marker and are unaffected.
func LooksLikeAbsenceMetaStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	inquiry := false
	for _, p := range []string{"用户询问", "用户问", "user asked", "user asks", "user inquired", "the user asked"} {
		if strings.Contains(s, p) {
			inquiry = true
			break
		}
	}
	if !inquiry {
		return false
	}
	for _, p := range []string{
		"暂无", "尚无", "没有相关记录", "无相关记录", "没有记录", "不在记忆", "未找到", "未存储", "没有保存",
		"需要用户提供", "待用户补充", "需要用户补充",
		"no record", "not in memory", "not on file", "don't have", "do not have", "no information", "not stored",
	} {
		if strings.Contains(s, p) {
			return true
		}
	}
	return false
}

// LooksLikePreferenceStatement reports durable likes, favorites, and residence.
func LooksLikePreferenceStatement(statement string) bool {
	s := strings.TrimSpace(statement)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "喜欢") || strings.Contains(low, "偏好") ||
		strings.Contains(low, "prefer") || strings.Contains(low, "favorite") ||
		strings.Contains(low, "favourite") {
		return true
	}
	return likeStatementRe.MatchString(s) || favoriteStatementRe.MatchString(s) || liveStatementRe.MatchString(s)
}

// LooksLikeConstraintStatement reports explicit rules. "always" alone is not
// a constraint ("I always drink coffee" is a preference/habit). Conditional
// and policy phrasings (如果/每当/只要/规定/规则/whenever/if…then/policy)
// count as constraints (AMC C3-A Q7a).
// AMC C3 修复 #2：原裸字串「若」（若干/若隐若现）与「按照」（按照计划执行 =
// 程序性陈述而非规则）误伤面太大，已移出约束 cue 清单——规则归类的召回
// 服务 G 维度，宁可少收不可把普通句误判为 evergreen 高权重 constraint
// （importance 0.9 会压制真实偏好事实）。
func LooksLikeConstraintStatement(statement string) bool {
	s := strings.ToLower(strings.TrimSpace(statement))
	if s == "" {
		return false
	}
	for _, p := range []string{"必须", "禁止", "不要", "must ", "never ", "do not ", "don't ", "required", "rule:"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	for _, p := range []string{"如果", "每当", "只要", "规定", "规则", "whenever", "policy"} {
		if strings.Contains(s, p) {
			return true
		}
	}
	if strings.Contains(s, "if ") && strings.Contains(s, "then") {
		return true
	}
	return false
}

// IsHypotheticalStatement reports hypothetical/contrafactive statements
// (如果…就/假如…则/要是…便/if…then…would) — they may classify as constraints
// for retrieval (G 维度保留) but must NEVER supersede a live fact
// (AMC C3 修复 #2): 「如果我搬到上海就退租」不能使「我住在杭州」失效。
func IsHypotheticalStatement(statement string) bool {
	s := strings.TrimSpace(statement)
	if s == "" {
		return false
	}
	low := strings.ToLower(s)
	outcomeCN := strings.Contains(s, "就") || strings.Contains(s, "则") || strings.Contains(s, "便")
	if evalHypoConditionMarker(s) && outcomeCN {
		return true
	}
	condEN := strings.HasPrefix(low, "if ") || strings.Contains(low, " if ")
	outcomeEN := strings.Contains(low, " then ") || strings.Contains(low, "then ") ||
		strings.Contains(low, "would ") || strings.Contains(low, "'ll ")
	return condEN && outcomeEN
}

// evalHypoConditionMarker reports a condition marker anywhere in s
// (如果/假如/要是/倘若/假若/if-clause). Shared by the hypothetical predicate
// and the round-3 clause/atom helpers below.
func evalHypoConditionMarker(s string) bool {
	for _, c := range []string{"如果", "假如", "要是", "倘若", "假若"} {
		if strings.Contains(s, c) {
			return true
		}
	}
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "if ") || strings.Contains(low, " if ")
}

// evalHypoOutcomeLead reports an outcome lead at the START of an atom —
// (我)就/(我)则/(我)便/then/would/'ll — the signature of a cross-sentence
// hypothetical whose condition sentence was split away by atomization
// (AMC P3 评审 R2-C6-2)。「如果P。我就Q了。」的主语+就 是结果句语法：
// 只查前 3 个字符内的 就/则/便——条件无关的日常句「我今天就走了8000步」
// 头窗无 就，不被误判合并而丢事实。
func evalHypoOutcomeLead(s string) bool {
	t := strings.TrimSpace(s)
	runes := []rune(t)
	w := 3
	if len(runes) < w {
		w = len(runes)
	}
	head := string(runes[:w])
	for _, o := range []string{"就", "则", "便"} {
		if strings.Contains(head, o) {
			return true
		}
	}
	low := strings.ToLower(t)
	return strings.HasPrefix(low, "then") || strings.Contains(low, " would ") || strings.Contains(low, "'ll ")
}

// evalHypoOutcomeMarker reports an outcome marker anywhere in a clause
// (就/则/便/would/'ll) — only treated as counterfactual AFTER a condition
// fragment was already dropped (ordered dependency): 「我七点就起床了，如果
// 明天下雨就取消晨跑」第一句的 就 是习惯副词不是结果标记，条件出现之前
// 的从句一律保留事实面（AMC P3 评审 R2-C6-1 的定向收紧）。
func evalHypoOutcomeMarker(p string) bool {
	if evalHypoOutcomeLead(p) {
		return true
	}
	for _, o := range []string{"就", "则", "便"} {
		if strings.Contains(p, o) {
			return true
		}
	}
	low := strings.ToLower(p)
	return strings.Contains(low, "would ") || strings.Contains(low, "'ll ")
}

// StripEvalHypotheticalClauses keeps the factual remainder of a mixed
// statement while dropping its hypothetical clauses (AMC P3 评审 R2-C6-1):
// 「我今天走了8000步，如果状态好明天就走10000步」→「我今天走了8000步」——真
// 实观测进序列/档案，反事实尾句不留。规则：整句已知假设时，条件从句
// （含 如果/if）与其后的结果从句（含 就/则/便/would）剔除，此前的事实
// 从句保留。从句边界是逗号（，/,)：句终标点不拆——跨句「如果…。就…。」
// 由 MergeEvalHypotheticalAtoms 在落库前合回整原子，这里的单原子视角下
// 整句假设、整体剔除。整句纯假设返回 ""。非假设句原样返回（恒等）。
func StripEvalHypotheticalClauses(statement string) string {
	s := strings.TrimSpace(statement)
	if s == "" || !IsHypotheticalStatement(s) {
		return s
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '，' || r == ',' })
	kept := make([]string, 0, len(parts))
	condSeen := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if IsHypotheticalStatement(p) || evalHypoConditionMarker(p) {
			condSeen = true
			continue
		}
		if condSeen && evalHypoOutcomeMarker(p) {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "，")
}

// MergeEvalHypotheticalAtoms rejoins a condition sentence with its outcome
// sentence after atomization (AMC P3 评审 R2-C6-2): 「如果明天下雨。/我就不
// 跑10公里了。」→ 一枚整体假设原子。原子化按句终标点切分会把条件标记剥
// 到上一枚，单独的「我就不跑10公里了。」既过不了 IsHypotheticalStatement
// 的联合判定又会以反事实数值播种序列/取值链；合并恢复联合上下文后，整
// 原子假设守卫在数值/rollup 三条链上统一拦截。
func MergeEvalHypotheticalAtoms(atoms []string) []string {
	if len(atoms) < 2 {
		return atoms
	}
	out := make([]string, 0, len(atoms))
	for _, a := range atoms {
		if len(out) > 0 && evalHypoOutcomeLead(a) && evalHypoConditionMarker(out[len(out)-1]) {
			out[len(out)-1] += a
			continue
		}
		out = append(out, a)
	}
	return out
}

// UserScopedFactKind reports whether the kind belongs on the user recall
// scope (cross-session), aligned with memory_remember.
func UserScopedFactKind(factKind string) bool {
	switch CanonicalizeFactKind(factKind, "") {
	case "user_identity", "preference", "constraint":
		return true
	}
	return false
}

// PreferenceSlotKey is a coarse attribute key used to detect same-slot
// updates (favorite color, residence, name). Empty means the statement is
// not a governable slot.
func PreferenceSlotKey(statement string) string {
	s := strings.TrimSpace(statement)
	if m := favoriteStatementRe.FindStringSubmatch(s); len(m) > 0 {
		// Re-parse the attribute word: `favorite <attr> is`.
		low := strings.ToLower(s)
		idx := strings.Index(low, "favorite")
		if idx < 0 {
			idx = strings.Index(low, "favourite")
		}
		if idx >= 0 {
			rest := strings.TrimSpace(s[idx:])
			fields := strings.Fields(rest)
			if len(fields) >= 2 {
				return "favorite:" + strings.ToLower(fields[1])
			}
		}
		return "favorite"
	}
	if liveStatementRe.MatchString(s) {
		return "live"
	}
	if LooksLikeUserIdentityStatement(s) || nameStatementRe.MatchString(s) {
		return "name"
	}
	if m := chineseFavoriteRe.FindStringSubmatch(s); len(m) > 1 {
		return "favorite:" + strings.ToLower(strings.TrimSpace(m[1]))
	}
	if strings.Contains(s, "住在") || strings.Contains(s, "居住在") {
		return "live"
	}
	// AMC C3-A Q7c slot extensions: residence moves, job changes and phone
	// numbers are governable slots so a later update supersedes the old value.
	lowS := strings.ToLower(s)
	if strings.Contains(s, "搬到") || strings.Contains(s, "搬去") || strings.Contains(lowS, "moved to") {
		return "live"
	}
	if strings.Contains(s, "换工作") || strings.Contains(s, "入职") || strings.Contains(s, "新工作") {
		return "job"
	}
	if strings.Contains(s, "手机号") || strings.Contains(s, "电话号码") {
		return "phone"
	}
	if likeStatementRe.MatchString(s) {
		return "like"
	}
	return ""
}

// HasPreferenceUpdateCue reports an explicit correction, not an additional
// preference (coffee and tea can coexist unless the user says "now/instead").
// AMC C3-A Q7c adds 不喜欢了/没感觉/改为 (改成/换成 were already covered).
func HasPreferenceUpdateCue(statement string) bool {
	low := strings.ToLower(statement)
	for _, n := range []string{" now", "now ", "instead", "no longer", "not anymore", "changed", "updated", "used to", "from now", "anymore", "现在", "改成", "换成", "不再", "不喜欢了", "没感觉", "改为"} {
		if strings.Contains(low, n) {
			return true
		}
	}
	return false
}

// ShouldSupersedeSameSlotFact reports whether an existing governable fact
// should be invalidated before writing the new one. Same-slot favorite /
// live / name updates always supersede; generic "like" updates require an
// update cue so "I like coffee" and "I like tea" can coexist. Hypothetical
// statements (如果…就/if…would) never supersede anything (AMC C3 修复 #2) —
// 「如果我搬到上海就退租」不是居住现状。
func ShouldSupersedeSameSlotFact(oldKind, oldStmt, newKind, newStmt string) bool {
	if !governableMemoryKind(oldKind) || !governableMemoryKind(newKind) {
		return false
	}
	if NormalizeForDedup(oldStmt) == NormalizeForDedup(newStmt) {
		return false
	}
	if IsHypotheticalStatement(newStmt) {
		return false
	}
	oldSlot, newSlot := PreferenceSlotKey(oldStmt), PreferenceSlotKey(newStmt)
	if oldSlot == "" || oldSlot != newSlot {
		return false
	}
	if oldSlot == "like" {
		return HasPreferenceUpdateCue(newStmt)
	}
	return true
}

func governableMemoryKind(kind string) bool {
	switch CanonicalizeFactKind(kind, "") {
	case "user_identity", "preference", "constraint", "agent_instruction":
		return true
	default:
		return false
	}
}
