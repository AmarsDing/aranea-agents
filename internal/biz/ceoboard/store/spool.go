// 事件 spool:落库失败时先写本地 JSONL,恢复后回填(自 ceo-dashboard 移植,近乎逐字)。
// 保真要点(风险点④崩溃恢复):
//   - 单条记录 sha256 校验,撕裂尾(崩溃写一半)隔离进 .quarantine;
//   - 导入前 .importing 改名认领(claim 只在短临界区改名,导入期间 append 可写新段);
//   - 段轮转(单段 16MiB)与总量上限(256MiB),超限返回 ErrSpoolFull;
//   - 每行写入后 fsync;目录权限 0700、文件 0600。
//
// 移植差异:DB 调用改走 Ent 仓储并传 ctx;文件操作原样保留。
package store

import (
	"bufio"
	"bytes"
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
	"strings"
	"sync"
	"time"

	"github.com/lib/pq"
)

const (
	defaultSpoolSegmentBytes int64 = 16 * 1024 * 1024
	defaultSpoolTotalBytes   int64 = 256 * 1024 * 1024
)

// ErrSpoolFull 表示 spool 总容量已满。
var ErrSpoolFull = errors.New("spool 总容量已满")

// IngestResult 是一次事件摄入的结果(直接落库 / 去重 / 转 spool)。
type IngestResult struct {
	Inserted  bool
	Duplicate bool
	Spooled   bool
}

// SpoolRecord 是 spool 单条记录(事件 + 失败原因 + 校验和)。
type SpoolRecord struct {
	Event     Event     `json:"event"`
	Reason    string    `json:"reason"`
	SpooledAt time.Time `json:"spooled_at"`
	Checksum  string    `json:"checksum"`
}

func (r SpoolRecord) canonicalWithoutChecksum() string {
	r.Checksum = ""
	raw, _ := json.Marshal(r)
	return string(raw)
}

type spool struct {
	dir          string
	segmentLimit int64
	totalLimit   int64
	mu           sync.Mutex
	importMu     sync.Mutex
	sequence     int
}

func newSpool(dir string, segmentLimit, totalLimit int64) (*spool, error) {
	if segmentLimit <= 0 {
		segmentLimit = defaultSpoolSegmentBytes
	}
	if totalLimit <= 0 {
		totalLimit = defaultSpoolTotalBytes
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("建 spool 目录: %w", err)
	}
	return &spool{dir: dir, segmentLimit: segmentLimit, totalLimit: totalLimit}, nil
}

// ConfigureSpool 启用事件 spool。
func (s *Store) ConfigureSpool(dir string, segmentLimit, totalLimit int64) error {
	sp, err := newSpool(dir, segmentLimit, totalLimit)
	if err != nil {
		return err
	}
	s.spool = sp
	return nil
}

// IngestEvent 先尝试落库;失败且配置了 spool 时转写本地,恢复后由 ImportSpool 回填。
func (s *Store) IngestEvent(ctx context.Context, ev *Event, cause error) (IngestResult, error) {
	inserted, err := s.InsertEvent(ctx, ev)
	if err == nil {
		return IngestResult{Inserted: inserted, Duplicate: !inserted}, nil
	}
	if s.spool == nil {
		return IngestResult{}, err
	}
	reason := err.Error()
	if cause != nil {
		reason = cause.Error() + "; " + reason
	}
	if spoolErr := s.spool.append(ev, errors.New(reason)); spoolErr != nil {
		s.spoolWriteFailed.Add(1)
		return IngestResult{}, fmt.Errorf("事件落库失败: %v; spool 写入失败: %w", err, spoolErr)
	}
	return IngestResult{Spooled: true}, nil
}

func (s *spool) append(ev *Event, cause error) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	rec := SpoolRecord{Event: *ev, Reason: reason, SpooledAt: time.Now().UTC()}
	sum := sha256.Sum256([]byte(rec.canonicalWithoutChecksum()))
	rec.Checksum = hex.EncodeToString(sum[:])
	raw, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if int64(len(raw)) > s.segmentLimit {
		return fmt.Errorf("单条 spool 记录超过段上限: %d > %d", len(raw), s.segmentLimit)
	}

	files, _, err := s.filesLocked()
	if err != nil {
		return err
	}
	path := ""
	// 单条记录不得超过段上限;追加会超限时轮转新段。
	if len(files) > 0 {
		path = files[len(files)-1]
		if fi, err := os.Stat(path); err == nil && fi.Size() > 0 {
			if err := repairTornTail(path, fi.Size()); err != nil {
				return err
			}
			fi, err = os.Stat(path)
			if err != nil {
				return err
			}
			if fi.Size()+int64(len(raw)) > s.segmentLimit {
				path = ""
			}
		} else {
			path = ""
		}
	}
	occupied, err := s.occupiedBytesLocked()
	if err != nil {
		return err
	}
	if occupied+int64(len(raw)) > s.totalLimit {
		return ErrSpoolFull
	}
	if path == "" {
		s.sequence++
		name := fmt.Sprintf("events-%s-%06d.jsonl", time.Now().UTC().Format("20060102T150405.000000000Z"), s.sequence)
		path = filepath.Join(s.dir, name)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func (s *spool) filesLocked() ([]string, int64, error) {
	files, err := filepath.Glob(filepath.Join(s.dir, "events-*.jsonl"))
	if err != nil {
		return nil, 0, err
	}
	sort.Strings(files)
	var total int64
	for _, path := range files {
		fi, err := os.Stat(path)
		if err != nil {
			return nil, 0, err
		}
		total += fi.Size()
	}
	return files, total, nil
}

func (s *spool) occupiedBytesLocked() (int64, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		total += info.Size()
	}
	return total, nil
}

func (s *spool) pendingBytes() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, total, err := s.filesLocked()
	if err != nil {
		return 0
	}
	return total
}

// ImportSpool 认领全部待导入段并逐行回填;损坏段隔离为 .quarantine 并返回错误。
func (s *Store) ImportSpool(ctx context.Context) (int, error) {
	if s.spool == nil {
		return 0, nil
	}
	sp := s.spool
	sp.importMu.Lock()
	defer sp.importMu.Unlock()

	// claim 只在短临界区内改名;落库等待期间 append 可写新段。
	sp.mu.Lock()
	claimed, err := filepath.Glob(filepath.Join(sp.dir, "events-*.jsonl.importing"))
	if err != nil {
		sp.mu.Unlock()
		return 0, err
	}
	files, _, err := sp.filesLocked()
	if err != nil {
		sp.mu.Unlock()
		return 0, err
	}
	for _, path := range files {
		claim := path + ".importing"
		if err := os.Rename(path, claim); err != nil {
			sp.mu.Unlock()
			return 0, err
		}
		claimed = append(claimed, claim)
	}
	sp.mu.Unlock()
	sort.Strings(claimed)

	imported := 0
	var poisonPaths []string
	for _, path := range claimed {
		records, torn, err := readSpoolSegment(path)
		if err != nil {
			if quarantineErr := quarantineSpool(path); quarantineErr != nil {
				return imported, fmt.Errorf("损坏段 %s: %v; 隔离失败: %w", path, err, quarantineErr)
			}
			return imported, fmt.Errorf("损坏段已隔离 %s: %w", path, err)
		}
		var poisoned []SpoolRecord
		for i := range records {
			if err := ctx.Err(); err != nil {
				return imported, err
			}
			inserted, err := s.insertEvent(ctx, &records[i].Event, true)
			if err != nil {
				// 数据类异常(SQLSTATE class 22,评审 2026-11-09 H3):该记录对
				// 当前 schema/编码是永久拒收,重试永无意义——单条隔离进
				// <段>.poison 后继续;否则它让整段 import 永久头阻塞
				// (pending 永不收敛,后续正常事件全部堵死)。
				if isPoisonDataErr(err) {
					poisoned = append(poisoned, records[i])
					continue
				}
				// 其他错误按瞬态(DB 故障)处理:整体中止,下个周期重试。
				return imported, err
			}
			if inserted {
				imported++
			}
		}
		if len(poisoned) > 0 {
			ppath, perr := appendPoisonRecords(path, poisoned)
			if perr != nil {
				return imported, fmt.Errorf("毒记录隔离 %s: %w", path, perr)
			}
			poisonPaths = append(poisonPaths, ppath)
		}
		if len(torn) > 0 {
			if err := writeTornQuarantine(path, torn); err != nil {
				return imported, err
			}
		}
		if err := os.Remove(path); err != nil {
			return imported, err
		}
	}
	if len(poisonPaths) > 0 {
		return imported, fmt.Errorf("spool 毒记录已隔离(不重试,需人工核查): %s", strings.Join(poisonPaths, ", "))
	}
	return imported, nil
}

// isPoisonDataErr 判定 SQLSTATE class 22(data exception)——持久性数据错误
// (非法 UTF-8/NUL 22021、值超长 22001 等),重投无意义。消毒(store.go
// sanitizeText)已消除 22021 主类,本判定拦截残余类。lib/pq 驱动。
func isPoisonDataErr(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code.Class()) == "22"
	}
	return false
}

// appendPoisonRecords 把永久拒收的记录追加为 <段>.poison(JSONL,0600),
// 返回文件路径。与 .quarantine(撕裂尾)分名:poison=语义拒收,需人工核查。
func appendPoisonRecords(claimPath string, recs []SpoolRecord) (string, error) {
	path := claimPath + ".poison"
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	for i := range recs {
		raw, err := json.Marshal(recs[i])
		if err != nil {
			return "", err
		}
		if _, err := f.Write(append(raw, '\n')); err != nil {
			return "", err
		}
	}
	return path, f.Sync()
}

func readSpoolSegment(path string) ([]SpoolRecord, []byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	var records []SpoolRecord
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if line[len(line)-1] != '\n' {
				if readErr != io.EOF {
					return nil, nil, readErr
				}
				return records, bytes.Clone(line), nil
			}
			var rec SpoolRecord
			if err := json.Unmarshal(bytes.TrimSpace(line), &rec); err != nil {
				return nil, nil, err
			}
			sum := sha256.Sum256([]byte(rec.canonicalWithoutChecksum()))
			if rec.Checksum != hex.EncodeToString(sum[:]) {
				return nil, nil, errors.New("checksum 不匹配")
			}
			records = append(records, rec)
		}
		if readErr == io.EOF {
			return records, nil, nil
		}
		if readErr != nil {
			return nil, nil, readErr
		}
	}
}

func quarantineSpool(path string) error {
	dst := path + ".quarantine"
	if _, err := os.Stat(dst); err == nil {
		dst = fmt.Sprintf("%s.%d.quarantine", path, time.Now().UnixNano())
	}
	return os.Rename(path, dst)
}

func repairTornTail(path string, size int64) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var last [1]byte
	_, readErr := f.ReadAt(last[:], size-1)
	closeErr := f.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if last[0] == '\n' {
		return nil
	}
	_, torn, err := readSpoolSegment(path)
	if err != nil {
		return err
	}
	if len(torn) == 0 {
		return errors.New("spool 非换行尾但未识别出撕裂数据")
	}
	if err := writeTornQuarantine(path, torn); err != nil {
		return err
	}
	return os.Truncate(path, size-int64(len(torn)))
}

func writeTornQuarantine(path string, raw []byte) error {
	dst := fmt.Sprintf("%s.torn-%d.quarantine", path, time.Now().UnixNano())
	f, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
