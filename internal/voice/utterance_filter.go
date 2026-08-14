package voice

import "unicode/utf8"

// utterance_filter.go — V11-T3 无意义终稿过滤（设计 §17.4）。
//
// 背景：listening 态任何非空 ASR 终稿都会建 Chat Turn——旁人说话、电视对白、
// 咳嗽叹息被识别后白建 Turn（烧 LLM token + 莫名回复）。本过滤器在确认词拦截
// 之后、状态机转换之前丢弃无意义终稿，状态停留 listening。
//
// 拦截链（handleASRFinal）：唤醒词剥离 → 退出词 → 确认词 → 【噪声过滤】→ Turn。

// noiseFillerWords 语气词/叹词表：整句精确匹配（归一化后）才丢弃。
// 与 internal/data/speech 的 ASR 热词表是两种用途（文本过滤 vs 识别增强），
// 各自演进；与 approveWords 的「嗯」重叠由拦截顺序裁决（confirm 在先）。
var noiseFillerWords = []string{
	"嗯", "嗯嗯", "啊", "啊啊", "呃", "呃呃", "哦", "哦哦", "噢",
	"唉", "哎", "喂", "那个", "唔",
	"hmm", "emm", "em", "uh", "um", "oh", "ah",
}

// minMeaningfulDurationMs 极短含混音时长下限：正常两字词实测 400ms+，
// 低于此值且 ≤2 rune 的终稿视为噪声碎片（F3）。
const minMeaningfulDurationMs = 300

// FilterNoiseFinal 判定 ASR 终稿是否应作为噪声丢弃（不建 Chat Turn）。
// durationMs ≤ 0 表示 Provider 未给时长（F3 不启用，宁留不杀）。
func FilterNoiseFinal(text string, durationMs int) (drop bool, reason string) {
	w := normalizeConfirmWord(text)
	if w == "" {
		return true, "empty" // 归一化后为空（纯标点/空白碎片）
	}
	for _, f := range noiseFillerWords {
		if w == f {
			return true, "filler_word"
		}
	}
	runes := utf8.RuneCountInString(w)
	if runes <= 1 {
		return true, "single_char"
	}
	if durationMs > 0 && durationMs < minMeaningfulDurationMs && runes <= 2 {
		return true, "too_short"
	}
	return false, ""
}
