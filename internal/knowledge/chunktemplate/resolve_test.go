package chunktemplate

import (
	"errors"
	"testing"
)

// 内置目录自检：≥8 条、全部合法、ID 唯一、兜底模板存在。
func TestBuiltinCatalog(t *testing.T) {
	ts := Builtin()
	if len(ts) < 8 {
		t.Fatalf("Builtin() len = %d, want >= 8", len(ts))
	}
	seen := make(map[string]struct{}, len(ts))
	hasDefault := false
	for _, tt := range ts {
		if err := tt.Validate(); err != nil {
			t.Errorf("template %q invalid: %v", tt.ID, err)
		}
		if _, ok := seen[tt.ID]; ok {
			t.Errorf("duplicate template ID %q", tt.ID)
		}
		seen[tt.ID] = struct{}{}
		if tt.ID == DefaultTemplateID {
			hasDefault = true
		}
	}
	if !hasDefault {
		t.Errorf("catalog missing default template %q", DefaultTemplateID)
	}
}

func TestResolve(t *testing.T) {
	r := DefaultResolver()
	cases := []struct {
		name     string
		docType  string
		mime     string
		filename string
		wantID   string
	}{
		// 任务验收用例
		{"chinese filename hits policy", "", "", "制度汇编.docx", "policy"},
		{"xlsx extension hits table", "", "", "数据表.xlsx", "table"},
		{"pdf without hints falls back to manual", "", "application/pdf", "scan.pdf", "manual"},
		{"unknown docType falls back to default", "unknown-xyz", "", "", "manual"},

		// 优先级：docType > MIME > 文件名/扩展名
		{"explicit docType wins over filename", "policy", "", "数据表.xlsx", "policy"},
		{"explicit docType wins over mime", "code", "text/csv", "", "code"},
		{"mime wins over filename", "", "text/csv", "用户手册.xlsx", "table"},

		// docType 匹配：大小写不敏感，支持中文
		{"docType case-insensitive", "Policy", "", "", "policy"},
		{"docType chinese exact", "制度", "", "", "policy"},
		{"docType derived faq", "faq", "", "", "faq"},
		{"docType derived report", "report", "", "", "report"},
		{"docType derived manual", "manual", "", "", "manual"},

		// MIME 匹配：剥掉参数、大小写不敏感
		{"csv mime hits table", "", "text/csv", "", "table"},
		{"xlsx mime hits table", "", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "", "table"},
		{"mime with charset param", "", "Text/CSV; charset=utf-8", "", "table"},

		// 文件名/扩展名匹配
		{"code extension", "", "", "main.go", "code"},
		{"code extension windows path", "", "", `src\app.py`, "code"},
		{"faq substring in filename", "", "", "FAQ.md", "faq"},
		{"contract substring in filename", "", "", "采购合同.pdf", "contract"},
		{"paper substring in filename", "", "", "毕业论文.docx", "paper"},
		{"manual substring in filename", "", "", "员工手册.pdf", "manual"},
		{"no hint md falls back", "", "", "readme.md", "manual"},
		{"all empty falls back", "", "", "", "manual"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := r.Resolve(tc.docType, tc.mime, tc.filename)
			if got.ID != tc.wantID {
				t.Fatalf("Resolve(%q, %q, %q).ID = %q, want %q",
					tc.docType, tc.mime, tc.filename, got.ID, tc.wantID)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("resolved template %q invalid: %v", got.ID, err)
			}
		})
	}
}

// 非法模板在 Validate 层报错。
func TestTemplateValidate(t *testing.T) {
	ok := Template{ID: "x", Strategy: StrategyMarkdown, ChunkSize: 512, Overlap: 64}
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid template rejected: %v", err)
	}

	cases := []struct {
		name string
		t    Template
		want error
	}{
		{"empty id", Template{ID: " ", Strategy: StrategyMarkdown, ChunkSize: 512}, ErrEmptyTemplateID},
		{"bad strategy", Template{ID: "x", Strategy: "bogus", ChunkSize: 512}, ErrInvalidStrategy},
		{"size too small", Template{ID: "x", Strategy: StrategyChar, ChunkSize: 127}, ErrInvalidChunkSize},
		{"size too big", Template{ID: "x", Strategy: StrategyChar, ChunkSize: 4097}, ErrInvalidChunkSize},
		{"negative overlap", Template{ID: "x", Strategy: StrategyChar, ChunkSize: 512, Overlap: -1}, ErrInvalidOverlap},
		{"overlap >= size", Template{ID: "x", Strategy: StrategyChar, ChunkSize: 512, Overlap: 512}, ErrInvalidOverlap},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.t.Validate(); !errors.Is(err, tc.want) {
				t.Fatalf("Validate() error = %v, want errors.Is %v", err, tc.want)
			}
		})
	}
}

// 构造期校验：空目录、缺兜底、ID 重复、含非法模板均报错。
func TestNewResolverErrors(t *testing.T) {
	valid := Builtin()
	if _, err := NewResolver(nil); !errors.Is(err, ErrNoTemplates) {
		t.Fatalf("NewResolver(nil) error = %v, want errors.Is %v", err, ErrNoTemplates)
	}

	noDefault := valid[1:] // Builtin[0] 即 manual
	if _, err := NewResolver(noDefault); !errors.Is(err, ErrNoDefaultTemplate) {
		t.Fatalf("NewResolver(noDefault) error = %v, want errors.Is %v", err, ErrNoDefaultTemplate)
	}

	dup := append(append([]Template{}, valid...), valid[1])
	if _, err := NewResolver(dup); !errors.Is(err, ErrDuplicateTemplateID) {
		t.Fatalf("NewResolver(duplicate ID) error = %v, want errors.Is %v", err, ErrDuplicateTemplateID)
	}

	badStrategy := append(append([]Template{}, valid...),
		Template{ID: "bad", Strategy: "bogus", ChunkSize: 512})
	if _, err := NewResolver(badStrategy); !errors.Is(err, ErrInvalidStrategy) {
		t.Fatalf("NewResolver(bad strategy) error = %v, want errors.Is %v", err, ErrInvalidStrategy)
	}
}

// 自定义目录（缩小版）同样走统一的优先级与兜底逻辑。
func TestResolverWithCustomCatalog(t *testing.T) {
	cat := []Template{
		{
			ID: DefaultTemplateID, Strategy: StrategyChar,
			ChunkSize: 512, Overlap: 64,
		},
		{
			ID: "spec", DocTypeHints: []string{"spec"}, MIMEHints: []string{"text/plain"},
			Strategy: StrategyToken, ChunkSize: 256, Overlap: 32,
		},
	}
	r, err := NewResolver(cat)
	if err != nil {
		t.Fatalf("NewResolver: %v", err)
	}
	if got := r.Resolve("spec", "", ""); got.ID != "spec" {
		t.Fatalf("docType resolve = %q, want spec", got.ID)
	}
	if got := r.Resolve("", "text/plain", ""); got.ID != "spec" {
		t.Fatalf("mime resolve = %q, want spec", got.ID)
	}
	if got := r.Resolve("别的", "", "无关.bin"); got.ID != DefaultTemplateID {
		t.Fatalf("fallback = %q, want %q", got.ID, DefaultTemplateID)
	}
}
