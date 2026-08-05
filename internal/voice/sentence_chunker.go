// Package voice implements the voice-session orchestration for the voice
// companion (M74): ASR ↔ Chat pipeline ↔ TTS glue, state machine, sentence
// chunking and TTS scheduling.
package voice

import (
	"regexp"
	"strings"
)

const (
	firstSentenceMinRunes = 6
	laterSentenceMinRunes = 12
	chunkHardMaxRunes     = 80
)

// SentenceChunker 将 LLM 流式 delta 切分为 TTS 句子（设计 §4.1）。
// 非并发安全：仅由会话事件循环单 goroutine 调用。
type SentenceChunker struct {
	onSentence func(text string, flush bool)
	buf        []rune
	emitted    int
	inFence    bool
	tickRun    int // 连续反引号计数（``` 栅栏检测）
	sinceMark  int // buf 中最近一个句子边界之后的位置（句长计量起点，短句向前合并）
}

func NewSentenceChunker(on func(text string, flush bool)) *SentenceChunker {
	return &SentenceChunker{onSentence: on}
}

func (c *SentenceChunker) Write(delta string) {
	for _, r := range delta {
		c.feedRune(r)
	}
	for c.runes() >= chunkHardMaxRunes {
		c.hardCut()
	}
}

// Flush 在 Turn 文本结束时强制送出残余（flush=true 标记尾句）。
func (c *SentenceChunker) Flush() {
	c.tickRun = 0 // 丢弃未闭合 fence 的残余反引号
	if len(c.buf) == 0 {
		return
	}
	c.cut(true)
}

func (c *SentenceChunker) runes() int { return len(c.buf) }

func (c *SentenceChunker) minRunes() int {
	if c.emitted == 0 {
		return firstSentenceMinRunes
	}
	return laterSentenceMinRunes
}

func (c *SentenceChunker) feedRune(r rune) {
	if r == '`' {
		c.tickRun++
		if c.tickRun == 3 {
			c.inFence = !c.inFence
			c.tickRun = 0
		}
		return
	}
	if c.tickRun > 0 {
		if !c.inFence {
			for i := 0; i < c.tickRun; i++ {
				c.buf = append(c.buf, '`')
			}
		}
		c.tickRun = 0
	}
	if c.inFence {
		return // 代码块内容不播报
	}
	c.buf = append(c.buf, r)
	if isSentenceBoundary(r) {
		if c.runes()-c.sinceMark >= c.minRunes() {
			c.cut(false)
			return
		}
		c.sinceMark = c.runes() // 短句不切，向前合并：句长从该边界重新计量
		return
	}
	// 首句优化：遇次要标点提前切（保首音延迟）
	if c.emitted == 0 && c.runes()-c.sinceMark >= firstSentenceMinRunes && isMinorBoundary(r) {
		c.cut(false)
	}
}

func (c *SentenceChunker) cut(flush bool) {
	text := cleanForSpeech(string(c.buf))
	c.buf = c.buf[:0]
	c.sinceMark = 0
	if text == "" {
		return
	}
	c.emitted++
	c.onSentence(text, flush)
}

// hardCut 达到硬上限时只送出头部 chunkHardMaxRunes 个字符，保留残余继续累积。
func (c *SentenceChunker) hardCut() {
	head := string(c.buf[:chunkHardMaxRunes])
	c.buf = c.buf[chunkHardMaxRunes:]
	if c.sinceMark > chunkHardMaxRunes {
		c.sinceMark -= chunkHardMaxRunes
	} else {
		c.sinceMark = 0
	}
	text := cleanForSpeech(head)
	if text == "" {
		return
	}
	c.emitted++
	c.onSentence(text, false)
}

func isSentenceBoundary(r rune) bool {
	switch r {
	case '。', '！', '？', '；', '\n':
		return true
	}
	return false
}

func isMinorBoundary(r rune) bool {
	switch r {
	case '，', ',', '、':
		return true
	}
	return false
}

var (
	urlRe   = regexp.MustCompile(`https?://\S+`)
	imgRe   = regexp.MustCompile(`!\[[^\]]*\]\([^)]*\)`)
	linkRe  = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
	tableRe = regexp.MustCompile(`(?m)^\s*\|.*$`)
	spaceRe = regexp.MustCompile(`\s{2,}`)
)

// cleanForSpeech 剥离不参与播报的 markdown 元素（URL/图片/表格/链接语法）。
// 注意顺序：图片/链接先于裸 URL 剥离，避免 urlRe 吞掉链接语法的右括号。
func cleanForSpeech(s string) string {
	s = imgRe.ReplaceAllString(s, "")
	s = linkRe.ReplaceAllString(s, "$1")
	s = urlRe.ReplaceAllString(s, "")
	s = tableRe.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "`", "")
	s = spaceRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
