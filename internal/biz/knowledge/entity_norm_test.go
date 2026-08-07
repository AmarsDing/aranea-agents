package knowledge

import "testing"

// NormalizeEntityName（B9）：NFKC（含全半形兼容）+ Unicode case-fold +
// 内部空白折叠为单空格 + 去首尾。验收："AI"/"ai"/"ＡＩ" 聚合为同一实体。
func TestNormalizeEntityName(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"ascii lower", "AI", "ai"},
		{"already lower", "ai", "ai"},
		{"fullwidth", "ＡＩ", "ai"},
		{"mixed fullwidth", "Ｍachine Ｌearning", "machine learning"},
		{"trim", "  RAG  ", "rag"},
		{"internal whitespace collapse", "large   language\tmodel", "large language model"},
		{"newline collapse", "knowledge\ngraph", "knowledge graph"},
		{"nbsp folds to space", "vector db", "vector db"},
		{"cjk unchanged", "知识图谱", "知识图谱"},
		{"cjk with whitespace", " 知识  图谱 ", "知识 图谱"},
		{"german case fold", "Straße", "strasse"},
		{"greek case fold", "ΟΣ", "οσ"},
		{"composed accent stable", "café", "café"},
		{"decomposed accent composes", "café", "café"},
		{"ligature folds", "ﬁle", "file"},
		{"empty", "", ""},
		{"only whitespace", "   ", ""},
		{"digit entity unchanged", "404", "404"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := NormalizeEntityName(c.in); got != c.want {
				t.Errorf("NormalizeEntityName(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
