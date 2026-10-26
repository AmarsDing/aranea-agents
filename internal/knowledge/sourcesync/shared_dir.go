// shared_dir.go —— G-38 W3 shared_dir walker：biz.SyncEngine.Scan 薄适配
// （设计 ②「IsIngestibleExt/32MB 白名单下沉实现侧，walk 与 filter 已分层」；
// ⑥-4 三态删除确认＝文件系统 walk-absent 即真相）。
//
// W3 as-built 登记（design 条文以条目注册；评审修轮以 #号标注）：
//   - external_id＝监视根内相对路径**逐字保留**（正斜杠，零映射——
//     「a/b.md vs a-b.md」防撞键钉设计即按构造免疫：不存在破坏性归一处；
//     出入两侧守卫＝**先验拒反斜杠/NUL**（#WALKER-2/W3-F3：SanitizeRelPath 是
//     改写非拒绝，代理会致 items 行坍缩张冠李戴）＋SanitizeRelPath 终审
//     拒 ../绝对路径/根内点目录；Walk 侧畸形名粒级 warn 跳过不杀全轮）。
//   - symlink/特殊文件围栏（#WALKER-1）：Scan 层 regular-file 闸（biz
//     SyncEngine 单点——WalkDir lstat 语义 + os.ReadFile 解析链接的非对称
//     不闸即根内投毒读根外，vault/shared_dir 两产线同封）；Fetch 再持
//     Lstat 闸关 Scan→Fetch 偷换 TOCTOU；ConfirmDeleted 对非正则文件
//     一律 Confirmed（identity 不存在=删除）。
//   - ConfirmDeleted 出席谓词回放（#W3-F2）：StillExists 唯「Scan 会出席」——
//     非正则/点文件/白名单外扩展/超 MaxBytes 的对象 identity 已死=Confirmed
//     （改名出白名单/超限不 limbo）；I/O 病与守卫拒绝=Unknown 保守不删。
//   - fingerprint＝文件内容 sha1（Scan 同源 HashContent 输出）——
//     文档 content_hash 与 adopt 判定同空间（Applier 侧以 payload 字节重算
//     自证，不信传输值）；mtime/size 只作 Scan 粗筛不进指纹（touch 不触发重摄取）。
//   - 游标 JSON {v,files:[{p,m,s,h}]}——mtime 预筛复用 hash 的唯一载体
//     （items 表只存 hash 不存 mtime）；游标损坏→空 prev 全量重 hash（成本
//     换不丢失，不触发误删：walk 仍出席全列）。容量上限随目录规模线性，
//     超大目录登记调参项（W4 接入面可不持久化游标退化为每轮全量 hash）。
//   - MIME 显式提交（Payload.MIME）——connector 文档 rel_path 置空后免疫
//     扩展名族 switch 的前提（设计 ① 身份寻址条）；文本类由 Applier 直读，
//     需抽取类（office/图片 MIME 表镜像 vault mimeTypeFor）走 Extractor 通道。
package sourcesync

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	bizsync "aranea-agents/internal/biz/knowledge/sourcesync"
	"aranea-agents/pkg/loggateway"
)

// SharedDirWalker shared_dir 唯一 Walker 实现（状态全在 deps/调用参数——可并发多实例）。
type SharedDirWalker struct {
	engine *bizknowledge.SyncEngine
	lg     loggateway.Logger
}

// NewSharedDirWalker 构造。Engine 为 nil 时自建（测试可注入统计/故障引擎）。
func NewSharedDirWalker(engine *bizknowledge.SyncEngine, lg loggateway.Logger) *SharedDirWalker {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	if engine == nil {
		engine = bizknowledge.NewSyncEngine(nil)
	}
	return &SharedDirWalker{engine: engine, lg: lg.With(loggateway.Domain("knowledge/sourcesync"))}
}

// Kind 注册键（runner Deps.Walkers 用同键）。
func (w *SharedDirWalker) Kind() bizsync.Kind { return bizsync.KindSharedDir }

// ── 游标编解码（v1 JSON；损坏即空 prev 全量重 hash） ──

type sharedDirCursorFile struct {
	Rel     string `json:"p"`
	ModNano int64  `json:"m"`
	Size    int64  `json:"s"`
	Hash    string `json:"h"`
}

type sharedDirCursor struct {
	V     int                   `json:"v"`
	Files []sharedDirCursorFile `json:"files,omitempty"`
}

func decodeSharedDirCursor(prev CursorState) []bizknowledge.FileSnapshot {
	if strings.TrimSpace(string(prev)) == "" {
		return nil
	}
	var cur sharedDirCursor
	if err := json.Unmarshal([]byte(prev), &cur); err != nil || cur.V != 1 {
		return nil // 坏游标：空 prev 全量重 hash（不失真，walk 出席全列）
	}
	out := make([]bizknowledge.FileSnapshot, 0, len(cur.Files))
	for _, f := range cur.Files {
		out = append(out, bizknowledge.FileSnapshot{
			RelPath: f.Rel,
			ModTime: time.Unix(0, f.ModNano).UTC(),
			Size:    f.Size,
			Hash:    f.Hash,
		})
	}
	return out
}

func encodeSharedDirCursor(snaps []bizknowledge.FileSnapshot) CursorState {
	cur := sharedDirCursor{V: 1, Files: make([]sharedDirCursorFile, 0, len(snaps))}
	for _, s := range snaps {
		cur.Files = append(cur.Files, sharedDirCursorFile{
			Rel: s.RelPath, ModNano: s.ModTime.UnixNano(), Size: s.Size, Hash: s.Hash,
		})
	}
	raw, err := json.Marshal(cur)
	if err != nil {
		return "" // 编排面契约：编码失败等价游标不动（调用侧按 error 收轮）
	}
	return CursorState(raw)
}

// sharedDirRootOf 配置解析+根目录活性校验（walker 侧唯一入口；config 缺陷归 walk_failed）。
func (w *SharedDirWalker) sharedDirRootOf(conn bizsync.Connector) (string, error) {
	cfg, err := bizsync.ParseSharedDirConfig(conn.ConfigJSON)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(cfg.Root)
	if err != nil {
		return "", fmt.Errorf("sourcesync: shared_dir root resolve: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", fmt.Errorf("sourcesync: shared_dir root stat: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("sourcesync: shared_dir root %q is not a directory", abs)
	}
	return abs, nil
}

// Walk ⑥-2：Scan 全量出席（出席即真相对账；runner 负责 needsWork/预算闸）。
// emit 出错即中止、游标返回 prev 原样（walker 契约——失败不存 checkpoint）。
func (w *SharedDirWalker) Walk(ctx context.Context, conn bizsync.Connector, prev CursorState, emit func(Snapshot) error) (CursorState, error) {
	root, err := w.sharedDirRootOf(conn)
	if err != nil {
		return prev, err
	}
	snaps, err := w.engine.Scan(root, decodeSharedDirCursor(prev))
	if err != nil {
		return prev, fmt.Errorf("sourcesync: shared_dir scan: %w", err)
	}
	for _, s := range snaps {
		if ctx.Err() != nil {
			return prev, ctx.Err()
		}
		ext, err := sanitizeExternalID(s.RelPath)
		if err != nil {
			// 粒级跳过不杀全轮（评审修 #WALKER-2/W3-F3：Linux 真含反斜杠的文件名
			// 无法用相对路径语义摄取——声明为不可摄取形态，warn 跳过；
			// Windows 无此名，此分支为跨平台防御）。
			w.lg.Warn("sourcesync: shared_dir skip unsanitizable entry",
				loggateway.Str("rel_path", s.RelPath), loggateway.Err(err))
			continue
		}
		if err := emit(Snapshot{
			ExternalID:  ext,
			Fingerprint: s.Hash,
			DisplayPath: s.RelPath,
		}); err != nil {
			return prev, err
		}
	}
	return encodeSharedDirCursor(snaps), nil
}

// Fetch ⑥ 提交面：打开监视根内文件（先守卫后落盘——sanitize 拒越界 +
// Lstat 正则文件闸，评审修 #WALKER-1：Scan→Fetch 符号链接偷换 TOCTOU 在此关门；
// Lstat→Open 残余窗接受为已知窗口，正则校验是主防线）。
func (w *SharedDirWalker) Fetch(ctx context.Context, conn bizsync.Connector, externalID string) (*Payload, error) {
	rel, err := sanitizeExternalID(externalID)
	if err != nil {
		return nil, err
	}
	root, err := w.sharedDirRootOf(conn)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	joined := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(joined)
	if err != nil {
		return nil, fmt.Errorf("sourcesync: fetch %q: %w", rel, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("sourcesync: fetch %q: not a regular file (symlink/special rejected)", rel)
	}
	f, err := os.Open(joined)
	if err != nil {
		return nil, fmt.Errorf("sourcesync: fetch %q: %w", rel, err)
	}
	info, err = f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("sourcesync: fetch stat %q: %w", rel, err)
	}
	return &Payload{Body: f, MIME: mimeForRelPath(rel), SizeBytes: info.Size()}, nil
}

// ConfirmDeleted ⑥-4 三态＋出席谓词回放（评审修 #W3-F2）：仍「Scan 会出席」
// 才算 StillExists——特殊文件（symlink/设备）/点文件/白名单外扩展/超 32MB
// 的对象 identity 已不存在（视同外部删除）=Confirmed；本身缺席=Confirmed；
// I/O 病/配置缺陷/守卫拒绝=Unknown（保守不删）。
func (w *SharedDirWalker) ConfirmDeleted(ctx context.Context, conn bizsync.Connector, externalID string) bizsync.DeletionVerdict {
	rel, err := sanitizeExternalID(externalID)
	if err != nil {
		return bizsync.VerdictUnknown
	}
	root, err := w.sharedDirRootOf(conn)
	if err != nil {
		return bizsync.VerdictUnknown
	}
	if ctx.Err() != nil {
		return bizsync.VerdictUnknown
	}
	// Lstat 对齐 Scan 出席谓词（WalkDir 不解析符号链接=lstat 语义）：symlink 指向
	// 根外常规文件若用 Stat 会误判 StillExists，与「Scan 会出席」回放口径相悖。
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		if os.IsNotExist(err) {
			return bizsync.VerdictConfirmed
		}
		return bizsync.VerdictUnknown
	}
	if !info.Mode().IsRegular() {
		return bizsync.VerdictConfirmed
	}
	base := filepath.Base(rel)
	if strings.HasPrefix(base, ".") || !bizknowledge.IsIngestibleExt(base) {
		return bizsync.VerdictConfirmed
	}
	if info.Size() > w.engine.MaxBytes() {
		return bizsync.VerdictConfirmed
	}
	return bizsync.VerdictStillExists
}

// sanitizeExternalID 出入唯一守卫：相对路径逐字保留（防撞键＝不映射），
// 拒绝对路径/../空/**反斜杠/NUL——扩张即地盘混淆，宁拒不歧义。
// 反斜杠先验拒（评审修 #WALKER-2/W3-F3）：biz SanitizeRelPath 首步是
// 反斜杠→正斜杠改写而不是拒绝，直接代理会让手改 items 行 docs\evil.md
// 坍缩到真档 docs/evil.md（张冠李戴）；故本层先拒，再交 SanitizeRelPath 终结审。
func sanitizeExternalID(rel string) (string, error) {
	if strings.ContainsAny(rel, "\\\x00") {
		return "", fmt.Errorf("sourcesync: external id %q rejected: backslash/NUL not allowed", rel)
	}
	clean, err := bizknowledge.SanitizeRelPath(rel)
	if err != nil {
		return "", fmt.Errorf("sourcesync: external id %q rejected: %w", rel, err)
	}
	return clean, nil
}

// mimeForRelPath 提交 MIME 表（镜像 vault mimeTypeFor 静态表——表新增扩展须双轨；
// 文本直读类（.md/.txt 等）由 biz.IsIngestibleExt 同源守卫先于本函数放行）。
func mimeForRelPath(rel string) string {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".pdf":
		return "application/pdf"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case ".xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case ".pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".webp":
		return "image/webp"
	default:
		// 文本直读家族（.md/.markdown/.txt/…）统一以 markdown 提交（frontmatter
		// 只属 vault 镜像语义；connector 通道正文即原文）。
		return "text/markdown"
	}
}
