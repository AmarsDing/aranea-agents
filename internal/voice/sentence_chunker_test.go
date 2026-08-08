package voice

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type sentence struct {
	text  string
	flush bool
}

type collector struct{ got []sentence }

func (c *collector) fn() func(string, bool) {
	return func(text string, flush bool) { c.got = append(c.got, sentence{text, flush}) }
}

func (c *collector) texts() []string {
	out := make([]string, 0, len(c.got))
	for _, s := range c.got {
		out = append(out, s.text)
	}
	return out
}

func TestChunkerBoundaryAndFlush(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("这是一个测试。下一句在这里。")
	require.Equal(t, []string{"这是一个测试。"}, c.texts())
	ch.Flush()
	require.Equal(t, []string{"这是一个测试。", "下一句在这里。"}, c.texts())
	require.True(t, c.got[1].flush)
}

func TestChunkerFirstSentenceMinorPunct(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("好的收到没问题，继续处理")
	require.Equal(t, []string{"好的收到没问题，"}, c.texts())
}

func TestChunkerFirstSentenceBelowMinMerges(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("好。继续说话。") // 首句 "好。" 仅 3 字符，不切
	require.Empty(t, c.texts())
	ch.Flush()
	require.Equal(t, []string{"好。继续说话。"}, c.texts())
}

func TestChunkerLaterSentenceMinMerge(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("第一句话已经够长了。短。也短。")
	// 首句切出；后续句 <12 字符合并
	require.Equal(t, []string{"第一句话已经够长了。"}, c.texts())
	ch.Flush()
	require.Equal(t, []string{"第一句话已经够长了。", "短。也短。"}, c.texts())
}

func TestChunkerHardCut(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write(strings.Repeat("字", 100))
	require.Len(t, c.got, 1)
	require.Equal(t, 80, len([]rune(c.got[0].text)))
	ch.Flush()
	require.Len(t, c.got, 2)
	require.Equal(t, 20, len([]rune(c.got[1].text)))
	require.True(t, c.got[1].flush)
}

func TestChunkerFenceStripped(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("好的```fmt.Println(1)```完成。")
	ch.Flush()
	require.Equal(t, []string{"好的完成。"}, c.texts())
}

func TestChunkerURLAndMarkdownStripped(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("看这个 https://example.com/x 即可。还有 [链接](https://a.b) 与 ![图](https://c.d) 都在。")
	ch.Flush()
	for _, s := range c.texts() {
		require.NotContains(t, s, "http")
		require.NotContains(t, s, "](")
	}
}

func TestChunkerNewlineBoundary(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	ch.Write("第一行足够长了吧\n第二行")
	require.Equal(t, []string{"第一行足够长了吧"}, c.texts())
}

func TestChunkerDropsPunctuationOnly(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	// 纯标点句无可读字符，喂给火山 TTS 会被拒（45002001 No readable text!）
	// 并计入连续失败（3 次中止调度器）——不下发。
	ch.Write("！！！。。。")
	ch.Flush()
	require.Empty(t, c.texts())
}

func TestChunkerKeepsSentenceWithLeadingPunct(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	// 前导标点 + 可读文本：整句保留（标点随可读句一起播报）。
	ch.Write("！！！好的没问题。")
	ch.Flush()
	require.Equal(t, []string{"！！！好的没问题。"}, c.texts())
}

func TestChunkerHardCutDropsPunctuationOnly(t *testing.T) {
	c := &collector{}
	ch := NewSentenceChunker(c.fn())
	// 纯标点达到硬上限：hardCut 同样丢弃。
	ch.Write(strings.Repeat("！", 100))
	require.Empty(t, c.texts())
	ch.Flush()
	require.Empty(t, c.texts())
}
