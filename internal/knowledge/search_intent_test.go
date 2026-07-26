package knowledge

import "testing"

func TestClassifySearchIntent(t *testing.T) {
	cases := []struct {
		q    string
		want SearchIntent
	}{
		// 即时区：路径/扩展名/引号短语
		{"notes/report.md", IntentInstant},
		{"财报\\2026", IntentInstant},
		{"q1 财报.pdf", IntentInstant},
		{"readme.md", IntentInstant},
		{"数据.xlsx", IntentInstant},
		{`"退款政策"`, IntentInstant},
		{`"exact phrase"`, IntentInstant},
		// 语义区：自然语言问句
		{"什么是知识库", IntentSemantic},
		{"如何配置 embedder？", IntentSemantic},
		{"为什么同步失败", IntentSemantic},
		{"怎么导入本地文件夹", IntentSemantic},
		{"哪些文档提到了退款", IntentSemantic},
		{"这个 vault 支持图片吗", IntentSemantic},
		{"how does sync work?", IntentSemantic},
		{"what is a vault", IntentSemantic},
		// 强即时信号优先于疑问词
		{"config.yaml 在哪", IntentInstant},
		// auto：无强信号
		{"退款政策", IntentAuto},
		{"embedding", IntentAuto},
		{"", IntentAuto},
		{"   ", IntentAuto},
	}
	for _, tc := range cases {
		if got := ClassifySearchIntent(tc.q); got != tc.want {
			t.Errorf("ClassifySearchIntent(%q) = %v, want %v", tc.q, got, tc.want)
		}
	}
}
