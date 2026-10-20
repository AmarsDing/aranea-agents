// Package chunktemplate 提供「文档类型 → 解析/切分参数」模板目录（U2-1）。
//
// 纯库：不依赖 biz/data/service、不读取外部配置；内置目录由 Builtin() 给出，
// Resolver 按 docType > MIME > 文件名/扩展名 优先级把一份待摄取文档解析到模板。
//
// 接线点（刻意后移，本包不做，避免与并行会话的 service 改动冲突）：
// internal/service/knowledge.go 的 IngestDocument 在请求未显式覆盖切分参数时
// （chunk_size=0、chunk_strategy=""、organize_to_markdown 未设置）调用
//
//	t := chunktemplate.DefaultResolver().Resolve(doc.DocType, req.GetMimeType(), req.GetSource())
//
// 并把模板映射回请求语义（api/kratos/knowledge/v1/knowledge.proto 的
// IngestDocumentRequest）：
//
//	chunk_strategy       ← t.Strategy（char|token|markdown|json|recursive）
//	chunk_size           ← t.ChunkSize
//	chunk_overlap        ← t.Overlap
//	organize_to_markdown ← t.OrganizeMarkdown
//
// 其中 doc.DocType 来自 biz/knowledge.DeriveSummaryCard 的摘要卡推导
// （report|manual|note|faq|…），mime_type 与 source 文件名来自请求本身。
package chunktemplate

import (
	"errors"
	"fmt"
	"strings"
)

// 切分策略取值，与 knowledge.ParseChunkStrategy 的可识别集合保持一致。
const (
	StrategyChar      = "char"
	StrategyToken     = "token"
	StrategyMarkdown  = "markdown"
	StrategyJSON      = "json"
	StrategyRecursive = "recursive"
)

// 切分参数合法区间：小于 MinChunkSize 召回过碎、大于 MaxChunkSize 超出嵌入
// 模型的有效上下文，均判非法。
const (
	MinChunkSize = 128
	MaxChunkSize = 4096
)

// DefaultTemplateID 解析兜底模板在目录中的 ID（Builtin 目录中的 manual）。
const DefaultTemplateID = "manual"

var (
	// ErrEmptyTemplateID 模板 ID 为空（或全空白）。
	ErrEmptyTemplateID = errors.New("chunktemplate: template ID is empty")
	// ErrInvalidStrategy 切分策略不在受支持集合内。
	ErrInvalidStrategy = errors.New("chunktemplate: unsupported chunk strategy")
	// ErrInvalidChunkSize 块大小超出 [MinChunkSize, MaxChunkSize]。
	ErrInvalidChunkSize = errors.New("chunktemplate: chunk size out of range")
	// ErrInvalidOverlap 重叠长度不满足 0 <= overlap < chunk size。
	ErrInvalidOverlap = errors.New("chunktemplate: overlap must satisfy 0 <= overlap < chunk size")
)

// Template 一份文档的解析/切分参数模板。
//
// 提示词匹配语义（Resolver 使用，详见 resolve.go）：
//   - DocTypeHints：文档类型词（如 "manual"、"制度"），与 docType 做大小写
//     不敏感精确匹配；非 "." 开头的条目同时参与文件名回退匹配——作为子串
//     （≥2 个字符，大小写不敏感）与文件名 basename 做包含判断；
//   - MIMEHints：MIME 类型（如 "text/csv"），与剥掉参数后的 mime 做大小写
//     不敏感精确匹配；以 "." 开头的条目是扩展名提示（如 ".xlsx"），与文件名
//     扩展名精确匹配。
type Template struct {
	ID               string
	Label            string
	Description      string
	DocTypeHints     []string
	MIMEHints        []string
	Strategy         string
	ChunkSize        int
	Overlap          int
	OrganizeMarkdown bool
}

// Validate 校验模板合法性：ID 非空；strategy ∈ {char, token, markdown, json,
// recursive}；chunk size ∈ [128, 4096]；0 <= overlap < chunk size。
func (t Template) Validate() error {
	if strings.TrimSpace(t.ID) == "" {
		return ErrEmptyTemplateID
	}
	switch t.Strategy {
	case StrategyChar, StrategyToken, StrategyMarkdown, StrategyJSON, StrategyRecursive:
	default:
		return fmt.Errorf("%w: %q (template %q)", ErrInvalidStrategy, t.Strategy, t.ID)
	}
	if t.ChunkSize < MinChunkSize || t.ChunkSize > MaxChunkSize {
		return fmt.Errorf("%w: size %d not in [%d, %d] (template %q)",
			ErrInvalidChunkSize, t.ChunkSize, MinChunkSize, MaxChunkSize, t.ID)
	}
	if t.Overlap < 0 || t.Overlap >= t.ChunkSize {
		return fmt.Errorf("%w: overlap %d, size %d (template %q)",
			ErrInvalidOverlap, t.Overlap, t.ChunkSize, t.ID)
	}
	return nil
}
