package knowledge

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseWikiLinks(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []string
	}{
		{name: "空正文", body: "", want: nil},
		{name: "无双链", body: "# 标题\n普通 [markdown](link) 不算", want: nil},
		{name: "基础形态", body: "参见 [[财报分析]] 一文", want: []string{"财报分析"}},
		{name: "带路径", body: "[[notes/2026/q2-report]]", want: []string{"notes/2026/q2-report"}},
		{name: "带 .md 后缀保留", body: "[[notes/a.md]]", want: []string{"notes/a.md"}},
		{name: "别名取目标", body: "[[目标文档|显示别名]]", want: []string{"目标文档"}},
		{name: "锚点剥离", body: "[[文档#章节]]", want: []string{"文档"}},
		{name: "路径+锚点+别名", body: "[[dir/doc#sec|别名]]", want: []string{"dir/doc"}},
		{name: "多个引用保序去重", body: "[[a]] 与 [[b]] 再到 [[a]]", want: []string{"a", "b"}},
		{name: "空引用跳过", body: "[[]] [[  ]] [[x]]", want: []string{"x"}},
		{name: "未闭合忽略", body: "[[未闭合 与 [[ok]]", want: []string{"ok"}},
		{name: "多行", body: "开头\n[[one]]\n中间 [[two]] 文本\n结尾", want: []string{"one", "two"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, ParseWikiLinks(tc.body))
		})
	}
}

func TestResolveLinkRefs(t *testing.T) {
	docs := []Document{
		{ID: "d1", RelPath: "notes/q2-report.md"},
		{ID: "d2", RelPath: "archive/旧闻.md"},
		{ID: "d3", RelPath: "readme.md"},
		{ID: "d4", RelPath: "a/dup.md"},
		{ID: "d5", RelPath: "b/dup.md"},
	}

	t.Run("精确路径匹配（含/不含 .md）", func(t *testing.T) {
		got := ResolveLinkRefs([]string{"notes/q2-report", "notes/q2-report.md"}, docs)
		assert.Equal(t, map[string]string{"notes/q2-report": "d1", "notes/q2-report.md": "d1"}, got)
	})
	t.Run("basename 匹配", func(t *testing.T) {
		got := ResolveLinkRefs([]string{"旧闻", "readme"}, docs)
		assert.Equal(t, map[string]string{"旧闻": "d2", "readme": "d3"}, got)
	})
	t.Run("basename 歧义取字典序首个（确定性）", func(t *testing.T) {
		got := ResolveLinkRefs([]string{"dup"}, docs)
		assert.Equal(t, map[string]string{"dup": "d4"}, got)
	})
	t.Run("悬空引用不建链", func(t *testing.T) {
		got := ResolveLinkRefs([]string{"不存在"}, docs)
		assert.Empty(t, got)
	})
	t.Run("大小写不敏感", func(t *testing.T) {
		got := ResolveLinkRefs([]string{"README"}, docs)
		assert.Equal(t, map[string]string{"README": "d3"}, got)
	})
	t.Run("空输入", func(t *testing.T) {
		assert.Empty(t, ResolveLinkRefs(nil, docs))
		assert.Empty(t, ResolveLinkRefs([]string{"x"}, nil))
	})
}
