package chunktemplate

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

var (
	// ErrNoTemplates 目录为空：解析无意义。
	ErrNoTemplates = errors.New("chunktemplate: resolver requires at least one template")
	// ErrNoDefaultTemplate 目录缺少兜底模板（DefaultTemplateID）：Resolve 无法保证全函数性。
	ErrNoDefaultTemplate = errors.New("chunktemplate: catalog missing default template")
	// ErrDuplicateTemplateID 目录内模板 ID 重复。
	ErrDuplicateTemplateID = errors.New("chunktemplate: duplicate template ID")
)

// Resolver 文档类型 → 切分模板的解析器。构造即完成全目录校验，之后并发安全
// （templates 不可变）。
type Resolver struct {
	templates []Template
	def       Template
}

// NewResolver 构造解析器：校验每个模板、要求 ID 唯一，且必须包含兜底模板
// （DefaultTemplateID，即 manual）。
func NewResolver(templates []Template) (*Resolver, error) {
	if len(templates) == 0 {
		return nil, ErrNoTemplates
	}
	seen := make(map[string]struct{}, len(templates))
	cps := make([]Template, len(templates))
	var def Template
	found := false
	for i, t := range templates {
		if err := t.Validate(); err != nil {
			return nil, err
		}
		if _, ok := seen[t.ID]; ok {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateTemplateID, t.ID)
		}
		seen[t.ID] = struct{}{}
		if t.ID == DefaultTemplateID {
			def, found = t, true
		}
		cps[i] = t
	}
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrNoDefaultTemplate, DefaultTemplateID)
	}
	return &Resolver{templates: cps, def: def}, nil
}

// DefaultResolver 返回基于内置目录的解析器。Builtin 目录的合法性由
// resolve_test 守护，此处的错误路径是防御性的不可达分支。
func DefaultResolver() *Resolver {
	r, err := NewResolver(Builtin())
	if err != nil {
		panic(fmt.Errorf("chunktemplate: builtin catalog invalid: %w", err))
	}
	return r
}

// Resolve 把一份待摄取文档解析到模板，优先级：
//
//  1. 显式 docType 命中某模板的 DocTypeHints（大小写不敏感精确匹配，支持中文）；
//  2. mime（剥掉 ";" 后的参数、小写）命中某模板的 MIMEHints（精确匹配）；
//  3. 文件名/扩展名：扩展名精确命中某模板的 "." 开头提示，或 basename 包含某
//     模板 DocTypeHints 中的非点号提示词（≥2 字符，大小写不敏感）；
//  4. 均未命中 → 兜底模板（manual）。
//
// 同一优先级内按目录顺序取首个命中。任意入参为空字符串 = 该级跳过，不算未命中。
func (r *Resolver) Resolve(docType, mime, filename string) Template {
	if dt := strings.ToLower(strings.TrimSpace(docType)); dt != "" {
		for _, t := range r.templates {
			for _, h := range t.DocTypeHints {
				if strings.HasPrefix(h, ".") {
					continue
				}
				if dt == strings.ToLower(h) {
					return t
				}
			}
		}
	}

	if m := normalizeMIME(mime); m != "" {
		for _, t := range r.templates {
			for _, h := range t.MIMEHints {
				if strings.HasPrefix(h, ".") {
					continue
				}
				if m == strings.ToLower(h) {
					return t
				}
			}
		}
	}

	if base, ext := splitFilename(filename); base != "" {
		for _, t := range r.templates {
			if matchFilename(t, base, ext) {
				return t
			}
		}
	}

	return r.def
}

// normalizeMIME 去掉参数部分并统一小写（"Text/CSV; charset=utf-8" → "text/csv"）。
func normalizeMIME(mime string) string {
	return strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
}

// splitFilename 取小写 basename 与扩展名；空名/纯路径返回 ("","")。
func splitFilename(filename string) (base, ext string) {
	base = strings.ToLower(filepath.Base(strings.TrimSpace(filename)))
	if base == "." || base == string(filepath.Separator) {
		return "", ""
	}
	return base, filepath.Ext(base)
}

// matchFilename 文件名/扩展名级匹配：扩展名提示（"." 开头，来自两类提示列表）与
// ext 精确匹配；DocTypeHints 中非点号提示词（≥2 字符）与 base 做包含匹配。
// ≥2 字符的阈值是为了把单字母提示排除在子串匹配外（扩展名单字母场景几乎总会误伤）。
func matchFilename(t Template, base, ext string) bool {
	for _, h := range t.DocTypeHints {
		h = strings.ToLower(strings.TrimSpace(h))
		if strings.HasPrefix(h, ".") {
			if ext != "" && h == ext {
				return true
			}
			continue
		}
		if utf8.RuneCountInString(h) >= 2 && strings.Contains(base, h) {
			return true
		}
	}
	if ext != "" {
		for _, h := range t.MIMEHints {
			h = strings.ToLower(strings.TrimSpace(h))
			if strings.HasPrefix(h, ".") && h == ext {
				return true
			}
		}
	}
	return false
}
