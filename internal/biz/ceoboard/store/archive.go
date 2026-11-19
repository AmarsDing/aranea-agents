// UTC+8 闭日归档(自 ceo-dashboard/internal/store/archive.go 移植,文件层近乎逐字)。
// 保真要点(风险点③):
//   - 按 UTC+8 自然日闭日;热数据只保留 retentionDays 天;
//   - 归档文件 events-YYYY-MM-DD.jsonl.gz + sha256 清单(.tmp 两阶段发布);
//   - 迟到保护:闭日已有归档却出现新事件 / 归档含截断原文但热数据已补齐 → 拒绝删除,
//     保留热数据并返回首个迟到错误;
//   - 崩溃续作:上次可能发布归档后、删热数据前崩溃——校验 UID 与完整性后续删;
//   - 删除守卫:DELETE 带 payload_complete 条件且要求影响行数==1,否则整体回滚。
//
// 移植差异:查询/删除改走 Ent(删除守卫用 Exec 影响行数);文件操作原样保留。
package store

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/ceoboardevent"
)

var archiveZone = time.FixedZone("UTC+8", 8*3600)

type archiveManifest struct {
	File   string `json:"file"`
	SHA256 string `json:"sha256"`
	Rows   int    `json:"rows"`
}

// ConfigureArchive 设置归档目录与热数据保留天数;capacity 为容量告警基线(字节)。
func (s *Store) ConfigureArchive(dir string, retentionDays int, capacity ...int64) error {
	if retentionDays <= 0 {
		retentionDays = 90
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("建归档目录: %w", err)
	}
	s.archiveDir = dir
	s.retentionDays = retentionDays
	if len(capacity) > 0 && capacity[0] > 0 {
		s.dbCapacityBytes = capacity[0]
	}
	return nil
}

// ArchiveClosedDays 归档所有已闭日的热数据;返回 (归档日数, 首个迟到错误)。
func (s *Store) ArchiveClosedDays(ctx context.Context, now time.Time) (int, error) {
	if s.archiveDir == "" {
		return 0, errors.New("归档目录未配置")
	}
	localNow := now.In(archiveZone)
	currentStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, archiveZone)
	cutoff := currentStart.AddDate(0, 0, -(s.retentionDays - 1))
	rows, err := s.client.CeoboardEvent.Query().
		Where(ceoboardevent.ReceivedAtLT(cutoff.UTC())).
		Order(ent.Asc(ceoboardevent.FieldReceivedAt), ent.Asc(ceoboardevent.FieldID)).
		All(ctx)
	if err != nil {
		return 0, err
	}
	byDay := map[string][]Event{}
	for _, e := range rows {
		ev := eventToDTO(e)
		day := ev.ReceivedAt.In(archiveZone).Format("2006-01-02")
		byDay[day] = append(byDay[day], ev)
	}
	days := make([]string, 0, len(byDay))
	for day := range byDay {
		days = append(days, day)
	}
	sort.Strings(days)
	archived := 0
	var firstLateErr error
	for _, day := range days {
		if err := ctx.Err(); err != nil {
			return archived, err
		}
		events := byDay[day]
		path := filepath.Join(s.archiveDir, "events-"+day+".jsonl.gz")
		if _, err := os.Stat(path); err == nil {
			if _, manifestErr := os.Stat(path + ".sha256.json"); os.IsNotExist(manifestErr) {
				if err := repairArchiveManifest(path); err != nil {
					return archived, fmt.Errorf("修复缺失归档清单: %w", err)
				}
			} else if manifestErr != nil {
				return archived, manifestErr
			}
			archivedCompleteness, err := archivedPayloadCompleteness(path)
			if err != nil {
				return archived, err
			}
			late := false
			for _, ev := range events {
				archivedComplete, exists := archivedCompleteness[ev.UID]
				if !exists {
					if firstLateErr == nil {
						firstLateErr = fmt.Errorf("闭日 %s 已有归档但出现迟到事件,拒绝删除未归档数据", day)
					}
					late = true
					break
				}
				if ev.PayloadComplete && !archivedComplete {
					if firstLateErr == nil {
						firstLateErr = fmt.Errorf("闭日 %s 的归档含截断原文,保留已补齐热数据", day)
					}
					late = true
					break
				}
			}
			if late {
				continue
			}
			// 上次可能在发布归档后、删除热数据前崩溃;确认 UID 与原文完整性后续作。
			if err := s.deleteArchivedEvents(ctx, events); err != nil {
				return archived, err
			}
			archived++
			continue
		} else if !os.IsNotExist(err) {
			return archived, err
		}
		if err := writeArchive(path, events); err != nil {
			return archived, err
		}
		if err := verifyArchive(path); err != nil {
			return archived, fmt.Errorf("归档校验失败,热数据未删除: %w", err)
		}
		if err := s.deleteArchivedEvents(ctx, events); err != nil {
			return archived, err
		}
		archived++
	}
	return archived, firstLateErr
}

func writeArchive(path string, events []Event) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	zw := gzip.NewWriter(f)
	enc := json.NewEncoder(zw)
	for i := range events {
		if err := enc.Encode(&events[i]); err != nil {
			zw.Close()
			f.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := stageArchiveManifest(path, tmp, len(events)); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return os.Rename(path+".sha256.json.tmp", path+".sha256.json")
}

func stageArchiveManifest(path, dataPath string, rows int) error {
	sum, err := fileSHA256(dataPath)
	if err != nil {
		return err
	}
	manifest := archiveManifest{File: filepath.Base(path), SHA256: sum, Rows: rows}
	raw, _ := json.Marshal(manifest)
	manifestTmp := path + ".sha256.json.tmp"
	mf, err := os.OpenFile(manifestTmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := mf.Write(raw); err != nil {
		mf.Close()
		return err
	}
	if err := mf.Sync(); err != nil {
		mf.Close()
		return err
	}
	return mf.Close()
}

func repairArchiveManifest(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	zr, err := gzip.NewReader(f)
	if err != nil {
		f.Close()
		return err
	}
	rows := 0
	dec := json.NewDecoder(zr)
	for {
		var ev Event
		if err := dec.Decode(&ev); err == io.EOF {
			break
		} else if err != nil {
			zr.Close()
			f.Close()
			return err
		}
		rows++
	}
	if err := zr.Close(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := stageArchiveManifest(path, path, rows); err != nil {
		return err
	}
	return os.Rename(path+".sha256.json.tmp", path+".sha256.json")
}

func verifyArchive(path string) error {
	raw, err := os.ReadFile(path + ".sha256.json")
	if err != nil {
		return err
	}
	var manifest archiveManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return err
	}
	sum, err := fileSHA256(path)
	if err != nil {
		return err
	}
	if manifest.SHA256 != sum {
		return errors.New("归档 checksum 不匹配")
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// deleteArchivedEvents 在单事务内逐行删除;每行要求影响行数==1
// (payload_complete 条件防"归档后补齐"的单调升级被误删),否则整体回滚。
func (s *Store) deleteArchivedEvents(ctx context.Context, events []Event) error {
	s.eventWriteMu.Lock()
	defer s.eventWriteMu.Unlock()

	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	for _, ev := range events {
		n, err := tx.CeoboardEvent.Delete().
			Where(ceoboardevent.ID(ev.UID), ceoboardevent.PayloadCompleteEQ(ev.PayloadComplete)).
			Exec(ctx)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		if n != 1 {
			_ = tx.Rollback()
			return fmt.Errorf("热数据在归档快照后已变化,保留事件 %s", ev.UID)
		}
	}
	return tx.Commit()
}

func archivedPayloadCompleteness(path string) (map[string]bool, error) {
	if err := verifyArchive(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	dec := json.NewDecoder(zr)
	completeness := map[string]bool{}
	for {
		var ev Event
		if err := dec.Decode(&ev); err == io.EOF {
			return completeness, nil
		} else if err != nil {
			return nil, err
		}
		completeness[ev.UID] = ev.PayloadComplete
	}
}

// RestoreArchive 从归档文件回填事件(幂等);返回新插入条数。
// 注意:RestoreArchive 直调 insertEventWithFlags,刻意旁路归档查重(L6)——
// 恢复的对象就是归档本身,查重会把恢复误判为重复投递而全部拒收。
func (s *Store) RestoreArchive(ctx context.Context, path string) (int, error) {
	if err := verifyArchive(path); err != nil {
		return 0, err
	}
	f, err := os.Open(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return 0, err
	}
	defer zr.Close()
	dec := json.NewDecoder(zr)
	imported := 0
	for {
		var ev Event
		if err := dec.Decode(&ev); err == io.EOF {
			return imported, nil
		} else if err != nil {
			return imported, err
		}
		inserted, err := s.insertEventWithFlags(ctx, &ev, ev.ImportedFromSpool, ev.PayloadComplete)
		if err != nil {
			return imported, err
		}
		if inserted {
			imported++
		}
	}
}

// ---------- 归档后重投查重(评审 2026-11-09 L6) ----------
//
// 背景:热数据按 event_uid 主键去重,但归档是硬删除——已归档事件的 UID 从
// 主键窗口消失。JetStream 以 DeliverAll 重建 consumer(双跑/漂移重建)时,
// 早于保留期的消息被整段重放,归一化把 received_at 刷成 now,于是每条已归档
// 事件都像个全新事件被再次计数(L6)。修复取"归档内查重"(对现有迁移影响
// 为零,不增列不改表):摄入老信封事件时按 occurred_at 定位候选闭日,在归档
// 文件内精确查重。权衡(相对 tombstone 列方案):信封不带时间的事件不在覆盖
// 内(无法与全新事件区分,残余重复计数面);查重成本为惰性文件扫描,但只在
// 老信封流量上触发,正常摄入零成本。
//
// archiveDedupMaxDays 是惰性闭日索引的 FIFO 上限:重放按流序推进、同日消息
// 聚集,8 天窗口足以覆盖顺序重放;被淘汰的最旧日再探时重扫(归档文件
// 不可变,重扫幂等)。防全年无序回填把 UID 集合全量驻留内存。
const archiveDedupMaxDays = 8

// archiveDedupIndex 是归档文件的惰性 UID 索引:仅老信封事件摄入时按需加载
// 对应闭日的 UID 集合(文件不可变,加载一次长期有效)。
type archiveDedupIndex struct {
	mu    sync.Mutex
	days  map[string]map[string]struct{} // 闭日(YYYY-MM-DD) → 该日归档 UID 集
	order []string                       // 加载先后(FIFO);超上限淘汰队首
}

func newArchiveDedupIndex() *archiveDedupIndex {
	return &archiveDedupIndex{days: map[string]map[string]struct{}{}}
}

// has 经 load 惰性加载并查 day 的 UID 集。第二个返回值为 false 表示"该日
// 无从证明"(文件缺失或损坏)——绝不把无从证明当成命中(查重只容漏、不容错,
// 漏了还有热库主键兜底,错了会丢事件)。缺失日不缓存:归档发布晚于探测,
// 缓存空集会让随后到达的同日重投放行。
func (d *archiveDedupIndex) has(day, uid string, load func(string) (map[string]struct{}, error)) (bool, bool) {
	d.mu.Lock()
	if set, ok := d.days[day]; ok {
		d.mu.Unlock()
		_, hit := set[uid]
		return hit, true
	}
	d.mu.Unlock()

	set, err := load(day)
	if err != nil || set == nil {
		return false, false
	}
	d.mu.Lock()
	if _, exists := d.days[day]; !exists { // 并发加载竞态:先到先得(文件不可变,集合等价)
		d.days[day] = set
		d.order = append(d.order, day)
		for len(d.order) > archiveDedupMaxDays {
			delete(d.days, d.order[0])
			d.order = d.order[1:]
		}
	}
	d.mu.Unlock()
	_, hit := set[uid]
	return hit, true
}

// archivedEventExists 报告事件是否已落入归档。只在信封 occurred_at 老于归档
// 截止线时探测候选闭日(occurred 当日 ±1 天,容 occurred/received 跨午夜与
// 时钟偏移)——重投消息的信封时间是原始的,received_at 却被归一化为 now,
// 无法按 received 定位,故以 occurred_at 为锚。occurred_at 晚于截止线的事件
// 不可能已归档(归档只动 received_at < cutoff 的闭日,且 occurred ≤ received),
// 正常流量因此零成本。
func (s *Store) archivedEventExists(ev *Event) bool {
	if s.archiveDir == "" || s.archDedup == nil {
		return false
	}
	occ := ev.OccurredAt
	if occ.IsZero() {
		return false // 信封未带时间:无法与全新事件区分,不在查重覆盖内(残余面)
	}
	localNow := time.Now().In(archiveZone)
	currentStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, archiveZone)
	cutoff := currentStart.AddDate(0, 0, -(s.retentionDays - 1))
	occLocal := occ.In(archiveZone)
	if !occLocal.Before(cutoff) {
		return false
	}
	uid := sanitizeText(ev.UID) // 与入库口径一致:归档存的是消毒形态
	day := occLocal.Format("2006-01-02")
	t, err := time.ParseInLocation("2006-01-02", day, archiveZone)
	if err != nil {
		return false
	}
	for _, candidate := range []string{
		t.AddDate(0, 0, -1).Format("2006-01-02"),
		day,
		t.AddDate(0, 0, 1).Format("2006-01-02"),
	} {
		if hit, ok := s.archDedup.has(candidate, uid, s.loadArchiveDayUIDs); ok && hit {
			return true
		}
	}
	return false
}

// loadArchiveDayUIDs 扫描某日归档文件的全部 UID(清单校验先行);文件不存在
// 返回 (nil, nil),调用方据此区分"无归档可证"与"损坏/错误"(后者同样不确认,
// 查重退化为热库口径,待归档流程下次运行自我修复)。
func (s *Store) loadArchiveDayUIDs(day string) (map[string]struct{}, error) {
	path := filepath.Join(s.archiveDir, "events-"+day+".jsonl.gz")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := verifyArchive(path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	set := map[string]struct{}{}
	dec := json.NewDecoder(zr)
	for {
		var ev Event
		if err := dec.Decode(&ev); err == io.EOF {
			return set, nil
		} else if err != nil {
			return nil, err
		}
		set[ev.UID] = struct{}{}
	}
}
