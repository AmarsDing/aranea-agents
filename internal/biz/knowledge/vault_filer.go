package knowledge

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// managedFrontmatterKeys KB 受管 frontmatter 字段（R-1：KB 独占，用户同名字段不生效）。
var managedFrontmatterKeys = map[string]bool{
	"id": true, "title": true, "tags": true, "type": true,
	"summary": true, "summary_hash": true, "source": true, "created": true,
}

// DocFrontmatter KB 受管字段（摘要卡）。用户自定义字段存 VaultDoc.Extra。
type DocFrontmatter struct {
	ID          string
	Title       string
	Tags        []string
	Type        string
	Summary     string
	SummaryHash string
	Source      string
	Created     time.Time
}

// VaultDoc 一个 vault 文档：frontmatter（受管 + 用户）+ Markdown 正文。
type VaultDoc struct {
	Frontmatter DocFrontmatter
	Extra       map[string]any // 用户自定义 frontmatter 字段（原样保留）
	Body        string
}

// selfWriteMark 一条 KB 自写标记（R-2 watcher 回环防护）。
// hash 为写入内容 hash（删除标记为空）；deleted 区分写入/删除，供 watcher
// 按事件类型匹配消费。
type selfWriteMark struct {
	hash    string
	deleted bool
}

// selfWriteMarkCap 自写标记容量上限。标记是 watcher hint 的 best-effort 过滤器，
// 丢失安全（退化为正常事件处理）；超限整体清空防无界增长。
const selfWriteMarkCap = 4096

// VaultFiler KB 侧唯一写文件出口（P1-2）。
// 契约：路径 sanitize（R-6 基础版）+ 覆盖前备份到 .aranea/trash（R-1/R-6）+ 原子写入。
// P2-3：所有写/删操作打自写标记，供 watcher 过滤 KB 自身事件（回环防护）。
type VaultFiler struct {
	lg loggateway.Logger

	mu         sync.Mutex
	selfWrites map[string]selfWriteMark // sanitized relPath → mark
}

// NewVaultFiler 构造。lg 为 nil 时使用 Noop。
func NewVaultFiler(lg loggateway.Logger) *VaultFiler {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &VaultFiler{
		lg:         lg.With(loggateway.Domain("knowledge")),
		selfWrites: make(map[string]selfWriteMark),
	}
}

var windowsDrivePattern = regexp.MustCompile(`^[a-zA-Z]:`)

// SanitizeRelPath 归一并校验 vault 内相对路径（R-6）：
// 反斜杠归一为 `/`；拒绝空/绝对路径/盘符/`..` 穿越/`.aranea` 元数据目录。
func SanitizeRelPath(rel string) (string, error) {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "", apierror.BadRequest("knowledge", "vault: empty rel_path")
	}
	rel = strings.ReplaceAll(rel, `\`, `/`)
	if windowsDrivePattern.MatchString(rel) || strings.HasPrefix(rel, "/") {
		return "", apierror.BadRequest("knowledge", "vault: absolute path not allowed: %q", rel)
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return "", apierror.BadRequest("knowledge", "vault: path traversal not allowed: %q", rel)
	}
	if cleaned == ".aranea" || strings.HasPrefix(cleaned, ".aranea/") {
		return "", apierror.BadRequest("knowledge", "vault: .aranea is reserved: %q", rel)
	}
	return cleaned, nil
}

// resolve 将已 sanitize 的相对路径解析为 root 内绝对路径，并防御性校验包含关系。
func resolve(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", apierror.Internal("knowledge", "vault: resolve root").WithCause(err)
	}
	target := filepath.Join(rootAbs, filepath.FromSlash(rel))
	if !strings.HasPrefix(target, rootAbs+string(os.PathSeparator)) {
		return "", apierror.BadRequest("knowledge", "vault: path escapes root: %q", rel)
	}
	return target, nil
}

// WriteDoc 原子写入 .md（tmp + rename）；已存在时先备份旧版本到 .aranea/trash。
func (f *VaultFiler) WriteDoc(root, relPath string, doc *VaultDoc) error {
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return err
	}
	target, err := resolve(root, rel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(target); err == nil {
		if err := f.backupToTrash(root, target, rel); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return apierror.Internal("knowledge", "vault: mkdir").WithCause(err)
	}
	content := marshalVaultDoc(doc)
	tmp, err := os.CreateTemp(filepath.Dir(target), ".aranea-tmp-*")
	if err != nil {
		return apierror.Internal("knowledge", "vault: create tmp").WithCause(err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return apierror.Internal("knowledge", "vault: write tmp").WithCause(err)
	}
	if err := tmp.Close(); err != nil {
		return apierror.Internal("knowledge", "vault: close tmp").WithCause(err)
	}
	// Windows 下 rename 要求目标不存在；旧版本已在 trash 有备份。
	_ = os.Remove(target)
	if err := os.Rename(tmpName, target); err != nil {
		return apierror.Internal("knowledge", "vault: rename").WithCause(err)
	}
	f.markSelfWrite(rel, HashContent(content), false)
	f.lg.Debug("vault doc written", loggateway.Str("rel_path", rel))
	return nil
}

// ReadDoc 读取并解析 .md：frontmatter（受管字段 + 用户 Extra）+ 正文。
// 无 frontmatter 的纯 Markdown 整篇作为 Body。
func (f *VaultFiler) ReadDoc(root, relPath string) (*VaultDoc, error) {
	doc, _, err := f.ReadDocWithHash(root, relPath)
	return doc, err
}

// ReadDocWithHash 同 ReadDoc，并返回原始文件内容 hash（WriteDocCAS 的 expectedHash 来源）。
func (f *VaultFiler) ReadDocWithHash(root, relPath string) (*VaultDoc, string, error) {
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return nil, "", err
	}
	target, err := resolve(root, rel)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		return nil, "", apierror.Internal("knowledge", "vault: read %s", rel).WithCause(err)
	}
	return parseVaultDoc(string(data)), HashContent(string(data)), nil
}

// WriteDocCAS 写入前重读目标文件比对 expectedHash（R-1：写入前重读 hash，冲突留双份）。
//
// expectedHash 为调用方上次读取时的文件 hash（ReadDocWithHash 返回值）；
// 空串表示「期望文件不存在」（创建场景防并发覆盖）。
//
// 冲突判定（三选一即冲突）：
//  1. 文件存在但 hash 不匹配——KB 基于的版本已被外部修改；
//  2. expectedHash 为空但文件已存在——并发创建撞上用户同名文件；
//  3. expectedHash 非空但文件已消失——KB 基于的版本已被删除。
//
// 冲突语义（保守默认：留双份）：磁盘当前版本备份进 .aranea/trash，仍写入新版本，
// conflict=true 供调用方感知/告警。覆盖即备份是 WriteDoc 既有契约（R-6），不区分冲突与否。
func (f *VaultFiler) WriteDocCAS(root, relPath string, doc *VaultDoc, expectedHash string) (bool, error) {
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return false, err
	}
	target, err := resolve(root, rel)
	if err != nil {
		return false, err
	}
	conflict := false
	data, readErr := os.ReadFile(target)
	switch {
	case readErr == nil:
		if expectedHash == "" || HashContent(string(data)) != expectedHash {
			conflict = true
		}
	case os.IsNotExist(readErr):
		if expectedHash != "" {
			conflict = true
		}
	default:
		return false, apierror.Internal("knowledge", "vault: read for cas %s", rel).WithCause(readErr)
	}
	if err := f.WriteDoc(root, rel, doc); err != nil {
		return false, err
	}
	if conflict {
		f.lg.Warn("vault write conflict, both copies kept",
			loggateway.Str("rel_path", rel),
			loggateway.Str("expected_hash", expectedHash),
		)
	}
	return conflict, nil
}

// MoveToTrash 将文件移入 .aranea/trash/（R-2：不物理删除），同名冲突加时间戳去重。
func (f *VaultFiler) MoveToTrash(root, relPath string) (string, error) {
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return "", err
	}
	target, err := resolve(root, rel)
	if err != nil {
		return "", err
	}
	trashPath, err := uniqueTrashPath(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return "", apierror.Internal("knowledge", "vault: mkdir trash").WithCause(err)
	}
	if err := os.Rename(target, trashPath); err != nil {
		return "", apierror.Internal("knowledge", "vault: move to trash %s", rel).WithCause(err)
	}
	f.markSelfWrite(rel, "", true)
	f.lg.Debug("vault doc moved to trash", loggateway.Str("rel_path", rel))
	return trashPath, nil
}

// WriteTrashFromMirror 把 DB 镜像内容抢救写入 .aranea/trash（R-2：外部删除不丢数据）。
// 用于同步层检测到外部删除后、删除 DB 镜像前——此时文件已不在磁盘，
// 只能从镜像重建。不写 vault 原路径（避免复活用户主动删除的文件），
// 不打自写标记（trash 目录在 vault 事件域之外，Scan 已忽略 `.` 前缀目录）。
func (f *VaultFiler) WriteTrashFromMirror(root, relPath string, doc *VaultDoc) (string, error) {
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return "", err
	}
	trashPath, err := uniqueTrashPath(root, rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return "", apierror.Internal("knowledge", "vault: mkdir trash").WithCause(err)
	}
	if err := os.WriteFile(trashPath, []byte(marshalVaultDoc(doc)), 0o644); err != nil {
		return "", apierror.Internal("knowledge", "vault: write trash from mirror %s", rel).WithCause(err)
	}
	f.lg.Debug("vault mirror rescued to trash", loggateway.Str("rel_path", rel))
	return trashPath, nil
}

// DirInfo 一个子目录条目（G1-B1 树节点目录来源）。
type DirInfo struct {
	Name    string
	ModTime time.Time
}

// ListSubdirs 返回 root 下 relPrefix 目录的直接子目录（名称 + mtime，按名称排序）。
// 只含目录：文件与点开头目录（.aranea/.git 等）被排除。
// relPrefix 为空 = root 本身；前缀目录不存在返回空（外部删除竞态，非错误）。
func (f *VaultFiler) ListSubdirs(root, relPrefix string) ([]DirInfo, error) {
	var target string
	if strings.TrimSpace(relPrefix) == "" {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, apierror.Internal("knowledge", "vault: resolve root").WithCause(err)
		}
		target = abs
	} else {
		rel, err := SanitizeRelPath(relPrefix)
		if err != nil {
			return nil, err
		}
		resolved, err := resolve(root, rel)
		if err != nil {
			return nil, err
		}
		target = resolved
	}
	entries, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, apierror.Internal("knowledge", "vault: read dir %s", relPrefix).WithCause(err)
	}
	out := make([]DirInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, DirInfo{Name: e.Name(), ModTime: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// markSelfWrite 记录 KB 自写标记（内部调用，rel 必须已 sanitize）。
func (f *VaultFiler) markSelfWrite(rel, hash string, deleted bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.selfWrites) >= selfWriteMarkCap {
		f.selfWrites = make(map[string]selfWriteMark)
	}
	f.selfWrites[rel] = selfWriteMark{hash: hash, deleted: deleted}
}

// ConsumeSelfWrite 取出并删除指定路径的自写标记（一次性消费语义）。
// watcher 回环防护：watcher 收到事件时调用，命中标记说明事件源自 KB 自身
// 写/删操作，可安全跳过。ok=false 表示无标记（外部事件，正常处理）。
// 消费即删防 map 无界增长；标记丢失安全（仅退化为正常处理）。
func (f *VaultFiler) ConsumeSelfWrite(relPath string) (hash string, deleted bool, ok bool) {
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return "", false, false
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	m, ok := f.selfWrites[rel]
	if !ok {
		return "", false, false
	}
	delete(f.selfWrites, rel)
	return m.hash, m.deleted, true
}

// backupToTrash 覆盖前复制旧版本到 trash（R-1/R-6：保守默认，不丢用户数据）。
func (f *VaultFiler) backupToTrash(root, target, rel string) error {
	data, err := os.ReadFile(target)
	if err != nil {
		return apierror.Internal("knowledge", "vault: read for backup %s", rel).WithCause(err)
	}
	trashPath, err := uniqueTrashPath(root, rel)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return apierror.Internal("knowledge", "vault: mkdir trash").WithCause(err)
	}
	if err := os.WriteFile(trashPath, data, 0o644); err != nil {
		return apierror.Internal("knowledge", "vault: backup %s", rel).WithCause(err)
	}
	return nil
}

func uniqueTrashPath(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", apierror.Internal("knowledge", "vault: resolve root").WithCause(err)
	}
	trashDir := filepath.Join(rootAbs, ".aranea", "trash")
	base := filepath.FromSlash(rel)
	candidate := filepath.Join(trashDir, base)
	if _, err := os.Stat(candidate); os.IsNotExist(err) {
		return candidate, nil
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return filepath.Join(trashDir, fmt.Sprintf("%s.%d%s", stem, time.Now().UnixNano(), ext)), nil
}

// HashContent 计算内容 sha1（content_hash / summary_hash 用）。
func HashContent(s string) string {
	sum := sha1.Sum([]byte(s))
	return "sha1:" + hex.EncodeToString(sum[:])
}

// SummaryStale 判定摘要卡是否过期（P2-2）：summaryHash 为空（从未生成）或
// 不等于当前 body hash 均为过期。摘要对象仅 Body——frontmatter 自身变更
// （含 KB 写回摘要）不会使摘要过期，避免「写回 → 整文件 hash 变 → stale →
// 再生成」的无限循环。
func SummaryStale(body, summaryHash string) bool {
	return summaryHash == "" || HashContent(body) != summaryHash
}

// marshalVaultDoc 序列化：受管字段（非空才写）+ 用户 Extra（受管同名键被忽略，R-1）。
func marshalVaultDoc(doc *VaultDoc) string {
	fm := map[string]any{}
	m := doc.Frontmatter
	if m.ID != "" {
		fm["id"] = m.ID
	}
	if m.Title != "" {
		fm["title"] = m.Title
	}
	if len(m.Tags) > 0 {
		fm["tags"] = m.Tags
	}
	if m.Type != "" {
		fm["type"] = m.Type
	}
	if m.Summary != "" {
		fm["summary"] = m.Summary
	}
	if m.SummaryHash != "" {
		fm["summary_hash"] = m.SummaryHash
	}
	if m.Source != "" {
		fm["source"] = m.Source
	}
	if !m.Created.IsZero() {
		fm["created"] = m.Created.UTC().Format(time.RFC3339)
	}
	for k, v := range doc.Extra {
		if managedFrontmatterKeys[k] {
			continue
		}
		fm[k] = v
	}
	var b strings.Builder
	if len(fm) > 0 {
		yml, _ := yaml.Marshal(fm)
		b.WriteString("---\n")
		b.Write(yml)
		b.WriteString("---\n\n")
	}
	b.WriteString(doc.Body)
	return b.String()
}

// parseVaultDoc 解析 frontmatter 块（--- yaml ---）+ 正文。
func parseVaultDoc(content string) *VaultDoc {
	doc := &VaultDoc{Body: content}
	rest := strings.TrimPrefix(content, "---\n")
	if rest == content {
		rest = strings.TrimPrefix(content, "---\r\n")
		if rest == content {
			return doc
		}
	}
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return doc
	}
	ymlPart := rest[:end]
	body := rest[end+len("\n---"):]
	// 跳过分隔行后的换行与空行
	body = strings.TrimPrefix(body, "\r")
	body = strings.TrimPrefix(body, "\n")
	body = strings.TrimPrefix(body, "\n")

	raw := map[string]any{}
	if err := yaml.Unmarshal([]byte(ymlPart), &raw); err != nil {
		// frontmatter 损坏时整篇按正文处理（保守，不丢内容）
		return doc
	}
	doc.Body = body
	doc.Extra = map[string]any{}
	for k, v := range raw {
		if !managedFrontmatterKeys[k] {
			doc.Extra[k] = v
			continue
		}
		switch k {
		case "id":
			doc.Frontmatter.ID = strVal(v)
		case "title":
			doc.Frontmatter.Title = strVal(v)
		case "tags":
			doc.Frontmatter.Tags = strSlice(v)
		case "type":
			doc.Frontmatter.Type = strVal(v)
		case "summary":
			doc.Frontmatter.Summary = strVal(v)
		case "summary_hash":
			doc.Frontmatter.SummaryHash = strVal(v)
		case "source":
			doc.Frontmatter.Source = strVal(v)
		case "created":
			if t, err := time.Parse(time.RFC3339, strVal(v)); err == nil {
				doc.Frontmatter.Created = t
			}
		}
	}
	return doc
}

func strVal(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}

func strSlice(v any) []string {
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, strVal(it))
	}
	return out
}
