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
