package hostexec

import (
	"context"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// NormalizeUTF8 把宿主命令输出的原始字节串归一化为合法 UTF-8。
//
// 背景：vendored hostexec 的 readFrom 直接 string(chunk)，不做编码处理。
// 子进程输出非 UTF-8 字节时（中文 Windows 的 GBK、wmic/PowerShell 重定向的
// UTF-16、抓取的 legacy 页面等），非法 UTF-8 会沿三条路径扩散：
//  1. 模型可见结果：encoding/json 把非法字节替换为 U+FFFD（乱码）。
//  2. tool_invocations.output_preview：PG UTF-8 库拒收（22021），记录丢失。
//  3. .aranea/shell 日志文件：原始字节落盘，读回乱码。
//
// 框架禁改（FW-R1），故在业务层包装器统一归一化。策略：
//  1. 已是合法 UTF-8 → 原样返回（快路径，零分配）。
//  2. 带 BOM 的 UTF-16 → 按 BOM 解码。
//  3. 其余按 GB18030 解码（GBK/GB2312 超集，与 knowledge vault_filer 一致）；
//     GB18030 几乎不会失败，失败时兜底 U+FFFD 替换保证合法。
func NormalizeUTF8(s string) string {
	if s == "" || utf8.ValidString(s) {
		return s
	}
	b := []byte(s)
	if len(b) >= 2 {
		var enc encoding.Encoding
		switch {
		case b[0] == 0xFF && b[1] == 0xFE:
			enc = unicode.UTF16(unicode.LittleEndian, unicode.ExpectBOM)
		case b[0] == 0xFE && b[1] == 0xFF:
			enc = unicode.UTF16(unicode.BigEndian, unicode.ExpectBOM)
		}
		if enc != nil {
			if out, _, err := transform.String(enc.NewDecoder(), s); err == nil {
				return out
			}
		}
	}
	if out, err := simplifiedchinese.GB18030.NewDecoder().String(s); err == nil {
		return out
	}
	return strings.ToValidUTF8(s, "�")
}

type utf8NormToolSet struct {
	inner trpctool.ToolSet
}

// WrapUTF8Norm 归一化 hostexec 工具结果中的所有字符串字段为合法 UTF-8。
// 挂在业务层链最内层（vendored 之后、redacting/sessionEnhance 之前），
// 使下游模型可见结果、调用记录、日志文件同时受益。
func WrapUTF8Norm(ts trpctool.ToolSet) trpctool.ToolSet {
	if ts == nil {
		return nil
	}
	return &utf8NormToolSet{inner: ts}
}

func (w *utf8NormToolSet) Name() string {
	if w.inner == nil {
		return ""
	}
	return w.inner.Name()
}

func (w *utf8NormToolSet) Close() error {
	if w.inner == nil {
		return nil
	}
	return w.inner.Close()
}

func (w *utf8NormToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if w.inner == nil {
		return nil
	}
	raw := w.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		if ct, ok := t.(trpctool.CallableTool); ok {
			out[i] = &utf8NormCallable{inner: ct}
		} else {
			out[i] = t
		}
	}
	return out
}

type utf8NormCallable struct {
	inner trpctool.CallableTool
}

func (c *utf8NormCallable) Declaration() *trpctool.Declaration {
	if c.inner == nil {
		return nil
	}
	return c.inner.Declaration()
}

func (c *utf8NormCallable) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if c.inner == nil {
		return nil, nil
	}
	result, err := c.inner.Call(ctx, jsonArgs)
	m, ok := result.(map[string]any)
	if !ok {
		return result, err
	}
	for key, val := range m {
		if s, ok := val.(string); ok && s != "" && !utf8.ValidString(s) {
			m[key] = NormalizeUTF8(s)
		}
	}
	return m, err
}
