package knowledge

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

// FileSnapshot 单个 vault 文件的同步快照（P1-3）。
type FileSnapshot struct {
	RelPath string    // vault 内相对路径（正斜杠）
	ModTime time.Time // 文件 mtime
	Size    int64
	Hash    string // HashContent 输出（sha1:...）
}

// ChangeType 同步变更类型。
type ChangeType int

const (
	ChangeCreated ChangeType = iota
	ChangeModified
	ChangeDeleted
	ChangeMoved // 凭 hash 识别的移动/重命名（保留文档身份）
)

// ChangeEvent 一次文件系统变更。Moved 时 OldRelPath 为原路径。
type ChangeEvent struct {
	Type       ChangeType
	RelPath    string
	OldRelPath string
	Snapshot   FileSnapshot
}

const defaultMaxFileBytes = 32 << 20 // 32MB

// SyncEngine 单向轮询扫描器（P1-3）：文件系统 → 变更事件。
// 纯逻辑组件，不含调度循环（调度在 usecase 接线时接入）。
type SyncEngine struct {
	lg       loggateway.Logger
	maxBytes int64
	hashFile func(path string) (string, error) // 可注入（测试统计/故障注入）
}

// NewSyncEngine 构造。lg 为 nil 时使用 Noop。
func NewSyncEngine(lg loggateway.Logger) *SyncEngine {
	e := &SyncEngine{
		lg:       lg,
		maxBytes: defaultMaxFileBytes,
	}
	if e.lg == nil {
		e.lg = loggateway.NewNoop()
	}
	e.lg = e.lg.With(loggateway.Domain("knowledge"))
	e.hashFile = defaultHashFile
	return e
}

func defaultHashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return HashContent(string(data)), nil
}

// Scan 扫描 vault root，返回当前 .md 文件快照。
// 忽略规则：任何以 `.` 开头的目录/文件（含 .aranea）、非 .md、超过 maxBytes。
// prev 非空时：mtime+size 未变的文件复用旧 hash（mtime 预筛，避免全量重算）。
func (e *SyncEngine) Scan(root string, prev []FileSnapshot) ([]FileSnapshot, error) {
	prevByPath := make(map[string]FileSnapshot, len(prev))
	for _, p := range prev {
		prevByPath[p.RelPath] = p
	}
	var snaps []FileSnapshot
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if path != root && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || !strings.HasSuffix(strings.ToLower(name), ".md") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil // 扫描期间文件消失，跳过
		}
		if info.Size() > e.maxBytes {
			e.lg.Warn("vault file skipped: oversize",
				loggateway.Str("path", path), loggateway.Str("size", strconv.FormatInt(info.Size(), 10)))
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		snap := FileSnapshot{RelPath: rel, ModTime: info.ModTime(), Size: info.Size()}
		if p, ok := prevByPath[rel]; ok && p.Size == snap.Size && p.ModTime.Equal(snap.ModTime) && p.Hash != "" {
			snap.Hash = p.Hash // mtime 预筛：复用
		} else {
			h, err := e.hashFile(path)
			if err != nil {
				return nil // 读取失败跳过（下轮重试）
			}
			snap.Hash = h
		}
		snaps = append(snaps, snap)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].RelPath < snaps[j].RelPath })
	return snaps, nil
}

// DiffSnapshots 对比前后快照产出变更事件。
// 移动判定：created 的 hash 命中 deleted 的 hash（同内容）→ Moved，保留身份；
// 移动且内容变更 → 保守按 删除+新增 处理。
func DiffSnapshots(prev, curr []FileSnapshot) []ChangeEvent {
	prevByPath := make(map[string]FileSnapshot, len(prev))
	for _, p := range prev {
		prevByPath[p.RelPath] = p
	}
	currByPath := make(map[string]FileSnapshot, len(curr))
	for _, c := range curr {
		currByPath[c.RelPath] = c
	}

	var created, modified, deleted []FileSnapshot
	for _, c := range curr {
		p, ok := prevByPath[c.RelPath]
		switch {
		case !ok:
			created = append(created, c)
		case p.Hash != c.Hash:
			modified = append(modified, c)
		}
	}
	for _, p := range prev {
		if _, ok := currByPath[p.RelPath]; !ok {
			deleted = append(deleted, p)
		}
	}

	deletedByHash := make(map[string]string, len(deleted)) // hash → relPath
	for _, d := range deleted {
		if _, exists := deletedByHash[d.Hash]; !exists {
			deletedByHash[d.Hash] = d.RelPath
		}
	}
	movedFrom := map[string]bool{}

	var events []ChangeEvent
	for _, c := range created {
		if oldPath, ok := deletedByHash[c.Hash]; ok {
			events = append(events, ChangeEvent{
				Type: ChangeMoved, RelPath: c.RelPath, OldRelPath: oldPath, Snapshot: c,
			})
			movedFrom[oldPath] = true
			continue
		}
		events = append(events, ChangeEvent{Type: ChangeCreated, RelPath: c.RelPath, Snapshot: c})
	}
	for _, m := range modified {
		events = append(events, ChangeEvent{Type: ChangeModified, RelPath: m.RelPath, Snapshot: m})
	}
	for _, d := range deleted {
		if movedFrom[d.RelPath] {
			continue
		}
		events = append(events, ChangeEvent{Type: ChangeDeleted, RelPath: d.RelPath, Snapshot: d})
	}

	sort.Slice(events, func(i, j int) bool {
		if events[i].Type != events[j].Type {
			return events[i].Type < events[j].Type
		}
		return events[i].RelPath < events[j].RelPath
	})
	return events
}
