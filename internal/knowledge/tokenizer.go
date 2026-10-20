package knowledge

import (
	"sync"

	"github.com/tiktoken-go/tokenizer"
)

// cl100kCodec 包级单例：BPE 词表加载昂贵，禁止每请求 ForModel/Get。
var cl100kCodec = sync.OnceValues(func() (tokenizer.Codec, error) {
	return tokenizer.Get(tokenizer.Cl100kBase)
})

// encodeTokens 返回 text 的 cl100k token 子串序列；暴露为变量以便测试注入失败路径。
var encodeTokens = defaultEncodeTokens

// defaultEncodeTokens 用 cl100k_base BPE 编码；词表加载/编码失败返回 nil（降级信号，
// 由调用方回落 rune 窗口，与 trpc_build.go tokenizer 降级模式同型）。
func defaultEncodeTokens(text string) []string {
	codec, err := cl100kCodec()
	if err != nil {
		return nil
	}
	_, toks, err := codec.Encode(text)
	if err != nil {
		return nil
	}
	return toks
}

// CountTokens 返回 text 的 cl100k token 数；编码器不可用时代以 rune 计数（上限近似）。
func CountTokens(text string) int {
	if toks := encodeTokens(text); toks != nil {
		return len(toks)
	}
	return len([]rune(text))
}

// tokenOffsets 返回每个 token 的 byte 右边界（含 0 起始，长度 len(toks)+1）：
// text[offs[i]:offs[i+1]] 即第 i 个 token 的原文。BPE 要求 token 串顺序拼接恒等
// 于原文；不符（编码器行为异常或漂移）返回 nil，调用方降级 rune 窗口。
func tokenOffsets(text string, toks []string) []int {
	if toks == nil {
		return nil
	}
	offs := make([]int, len(toks)+1)
	for i, tk := range toks {
		offs[i+1] = offs[i] + len(tk)
	}
	if offs[len(toks)] != len(text) {
		return nil
	}
	return offs
}
