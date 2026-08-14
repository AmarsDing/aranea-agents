package monitor

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

const (
	// flowFileSelfStepPrefix marks log entries emitted by FlowFileAppender
	// itself. Such events must never be written back to the flow files:
	// a write failure would otherwise produce a Warn that re-enters this
	// appender via the MonitorBus and fails again, creating a self-feedback
	// log storm (observed in production on a full disk).
	flowFileSelfStepPrefix = "monitor.flow_file."

	// flowFileWriteFailThreshold is the number of consecutive encode failures
	// that trips the write circuit breaker.
	flowFileWriteFailThreshold = 3
	// flowFileWriteMuteDuration is how long writes stay muted after the
	// circuit breaker trips.
	flowFileWriteMuteDuration = time.Minute
	// flowFileMaxBackupsDefault caps rotated backup files (prefix-date.jsonl.N)
	// kept per base name. Without a cap, size-based rotation produces
	// unbounded files that retention never matched (suffix filter missed
	// ".jsonl.N"), eventually filling the disk.
	flowFileMaxBackupsDefault = 10
)

type FlowFileAppender struct {
	dir           string
	retentionDays int
	compressAge   time.Duration
	maxBackups    int

	mu         sync.Mutex
	flowFile   *rotatingFile
	systemFile *rotatingFile
	traceFile  *rotatingFile
	alertFile  *rotatingFile
	logFile    *rotatingFile
	lg         loggateway.Logger

	// Write-failure circuit breaker state (guarded by mu).
	writeFailStreak int
	writeMutedUntil time.Time
}

type rotatingFile struct {
	path     string
	file     *os.File
	encoder  *json.Encoder
	date     string
	maxSize  int64
	seq      int
	baseName string
}

func (rf *rotatingFile) currentPath() string {
	if rf.seq <= 1 {
		return rf.baseName
	}
	return fmt.Sprintf("%s.%d", rf.baseName, rf.seq)
}

func (rf *rotatingFile) shouldRotate() bool {
	if rf.maxSize <= 0 || rf.file == nil {
		return false
	}
	fi, err := rf.file.Stat()
	if err != nil {
		return false
	}
	return fi.Size() >= rf.maxSize
}

func NewFlowFileAppender(dir string, lg loggateway.Logger) *FlowFileAppender {
	if dir == "" {
		if runtime.GOOS == "windows" {
			dir = "./logs"
		} else {
			dir = "/var/log/aranea"
		}
	}
	return &FlowFileAppender{
		dir:           dir,
		retentionDays: 30,
		compressAge:   24 * time.Hour,
		maxBackups:    flowFileMaxBackupsDefault,
		lg:            lg,
	}
}

// Start subscribes the appender to a MonitorBus and launches the maintenance loop.
//
// Phase 5 Blocker B: migrated from legacy Envelope-based contract.Bus to
// typed contract.MonitorBus. The filter at the bus level accepts only the
// monitor event types this appender knows how to route (flow_log/log/alert.notify/
// mcp.health.alert), preventing non-matching events from filling the queue.
func (a *FlowFileAppender) Start(ctx context.Context, bus contract.MonitorBus) {
	if a == nil {
		return
	}
	if err := os.MkdirAll(a.dir, 0755); err != nil {
		a.lg.Warn("FlowFileAppender: mkdir failed", loggateway.StepID("monitor.flow_file.mkdir_fail"), loggateway.Str("dir", a.dir), loggateway.Err(err))
		return
	}
	if bus != nil {
		opts := contract.MonitorSubscribeOptions{
			BufferSize: 1024,
			GlobalMode: true,
			Filter: func(ev contract.MonitorEvent) bool {
				switch ev.Type {
				case contract.MonitorEventTypeFlowLog,
					contract.MonitorEventTypeLog,
					contract.MonitorEventTypeAlertNotify,
					contract.MonitorEventTypeMCPHealthAlert:
					return true
				}
				return false
			},
		}
		ch, unsub := bus.Subscribe(opts)
		safego.Go(ctx, "flow-file-appender", func() {
			defer unsub()
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-ch:
					if !ok {
						return
					}
					a.onMonitorEvent(ev)
				}
			}
		})
	}
	safego.Go(ctx, "flow-file-appender-maintenance", func() {
		a.maintenance()
		ticker := time.NewTicker(time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				a.maintenance()
			}
		}
	})
}

func (a *FlowFileAppender) maintenance() {
	a.syncOpenFiles()
	compressed := a.compressOldFiles()
	purged := a.purgeExpiredFiles()
	backupsPurged := a.purgeExcessBackups()
	a.purgeTmpFiles()
	if compressed > 0 || purged > 0 || backupsPurged > 0 {
		a.lg.Info("FlowFileAppender maintenance completed",
			loggateway.StepID("monitor.flow_file.maintenance"),
			loggateway.Str("compressed", fmt.Sprint(compressed)), loggateway.Str("purged", fmt.Sprint(purged)),
			loggateway.Str("backups_purged", fmt.Sprint(backupsPurged)))
	}
}

func (a *FlowFileAppender) syncOpenFiles() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range []*rotatingFile{a.flowFile, a.systemFile, a.traceFile, a.alertFile, a.logFile} {
		if f != nil && f.file != nil {
			f.file.Sync()
		}
	}
}

func (a *FlowFileAppender) compressOldFiles() int {
	if a.dir == "" || a.compressAge <= 0 {
		return 0
	}
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().UTC().Add(-a.compressAge)
	compressed := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UTC().After(cutoff) {
			continue
		}
		srcPath := filepath.Join(a.dir, name)
		dstPath := srcPath + ".gz"
		if _, err := os.Stat(dstPath); err == nil {
			continue
		}
		if a.compressFile(srcPath, dstPath) {
			compressed++
		}
	}
	return compressed
}

func (a *FlowFileAppender) compressFile(src, dst string) bool {
	tmpDst := dst + ".tmp"
	sf, err := os.Open(src)
	if err != nil {
		return false
	}
	defer sf.Close()
	df, err := os.Create(tmpDst)
	if err != nil {
		return false
	}
	gw := gzip.NewWriter(df)
	if _, err := io.Copy(gw, sf); err != nil {
		gw.Close()
		df.Close()
		os.Remove(tmpDst)
		return false
	}
	if err := gw.Close(); err != nil {
		df.Close()
		os.Remove(tmpDst)
		return false
	}
	if err := df.Sync(); err != nil {
		df.Close()
		os.Remove(tmpDst)
		return false
	}
	df.Close()
	if err := os.Rename(tmpDst, dst); err != nil {
		os.Remove(tmpDst)
		return false
	}
	os.Remove(src)
	return true
}

func (a *FlowFileAppender) purgeExpiredFiles() int {
	if a.dir == "" || a.retentionDays <= 0 {
		return 0
	}
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return 0
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -a.retentionDays)
	purged := 0
	for _, e := range entries {
		name := e.Name()
		if !isFlowLogFileName(name) {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().UTC().After(cutoff) {
			continue
		}
		if os.Remove(filepath.Join(a.dir, name)) == nil {
			purged++
		}
	}
	return purged
}

// isFlowLogFileName reports whether name belongs to the flow-log file family:
// base files (prefix-date.jsonl), compressed archives (.jsonl.gz), and
// size-rotated backups (.jsonl.N / .jsonl.N.gz). Rotated backups previously
// escaped retention because the plain ".jsonl"/".jsonl.gz" suffix check
// missed them, letting them accumulate until the disk filled.
func isFlowLogFileName(name string) bool {
	if strings.HasSuffix(name, ".jsonl") || strings.HasSuffix(name, ".jsonl.gz") {
		return true
	}
	rest := strings.TrimSuffix(name, ".gz")
	idx := strings.LastIndex(rest, ".jsonl.")
	if idx < 0 {
		return false
	}
	seq, err := strconv.Atoi(rest[idx+len(".jsonl."):])
	return err == nil && seq >= 2
}

// purgeExcessBackups caps the number of size-rotated backup files
// (prefix-date.jsonl.N, N >= 2) per base name at maxBackups, deleting the
// oldest excess ones. The base file (no .N suffix) is never touched.
func (a *FlowFileAppender) purgeExcessBackups() int {
	if a.dir == "" || a.maxBackups <= 0 {
		return 0
	}
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return 0
	}
	// Group rotated backups by base name (e.g. "flow-2026-08-13.jsonl").
	groups := make(map[string][]int)
	for _, e := range entries {
		name := e.Name()
		rest := strings.TrimSuffix(name, ".gz")
		idx := strings.LastIndex(rest, ".jsonl.")
		if idx < 0 {
			continue
		}
		seq, err := strconv.Atoi(rest[idx+len(".jsonl."):])
		if err != nil || seq < 2 {
			continue
		}
		base := rest[:idx+len(".jsonl")]
		groups[base] = append(groups[base], seq)
	}
	purged := 0
	for base, seqs := range groups {
		if len(seqs) <= a.maxBackups {
			continue
		}
		sort.Ints(seqs)
		for _, seq := range seqs[:len(seqs)-a.maxBackups] {
			name := fmt.Sprintf("%s.%d", base, seq)
			if os.Remove(filepath.Join(a.dir, name)) == nil {
				purged++
			}
			// Best-effort cleanup of a compressed sibling if present.
			os.Remove(filepath.Join(a.dir, name+".gz"))
		}
	}
	return purged
}

func (a *FlowFileAppender) purgeTmpFiles() {
	if a.dir == "" {
		return
	}
	entries, err := os.ReadDir(a.dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".gz.tmp") {
			os.Remove(filepath.Join(a.dir, name))
		}
	}
}

// onMonitorEvent routes a MonitorEvent to the appropriate file and writes it.
//
// Phase 5 Blocker B: migrated from legacy Envelope. The MonitorEvent.Type
// replaces EnvelopeType; the Message field replaces Envelope.Content.Text;
// the Timestamp (time.Time) replaces the legacy string timestamp; the Source
// field replaces the legacy Channel field for FlowLog routing.
//
// FlowLog routing (preserves legacy behavior):
//   - Source=="flow" → systemFile (FlowTracker emits with Source="flow",
//     equivalent to legacy Channel="monitor" which routed to systemFile)
//   - Other Source → flowFile (preserves legacy Channel=anything-else behavior)
func (a *FlowFileAppender) onMonitorEvent(ev contract.MonitorEvent) {
	if a == nil || ev.Metadata == nil {
		return
	}
	// Break the self-feedback loop: log entries emitted by this appender
	// (write/open/mkdir failures, maintenance summaries) ride the MonitorBus
	// back into this handler. Writing them again would fail again and emit
	// another Warn, producing an unbounded log storm. They are still
	// available in the process log (aranea-pipeline.log) and the DB, so
	// dropping them here loses nothing.
	if strings.HasPrefix(metaStr(ev.Metadata, "step_id"), flowFileSelfStepPrefix) {
		return
	}

	row := make(map[string]any, len(ev.Metadata)+4)
	for k, v := range ev.Metadata {
		row[k] = v
	}
	row["_ts"] = ev.Timestamp.UTC().Format(time.RFC3339Nano)
	row["_id"] = ev.ID
	row["_session_id"] = ev.SessionID
	if ev.Message != "" {
		row["_text"] = ev.Message
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	// Circuit breaker covers BOTH open and write failures: a bad dir / full
	// disk otherwise emits one unthrottled Warn per event (open_fail was a
	// blind spot — the breaker used to live only in writeRowLocked).
	if a.writeMutedLocked() {
		return
	}
	target := a.routeFileLocked(ev)
	if target != nil {
		a.writeRowLocked(target, row)
	}
}

func (a *FlowFileAppender) routeFileLocked(ev contract.MonitorEvent) *rotatingFile {
	switch ev.Type {
	case contract.MonitorEventTypeAlertNotify, contract.MonitorEventTypeMCPHealthAlert:
		return a.ensureFile(&a.alertFile, "alert")
	case contract.MonitorEventTypeLog:
		return a.ensureFile(&a.logFile, "log")
	case contract.MonitorEventTypeFlowLog:
		// Legacy FlowTracker set Channel="monitor" → systemFile. The typed
		// MonitorEvent has no Channel field; Source="flow" is the equivalent
		// marker emitted by FlowTracker, so we route Source="flow" to systemFile
		// to preserve production behavior.
		if strings.TrimSpace(ev.Source) == "flow" {
			return a.ensureFile(&a.systemFile, "system")
		}
		return a.ensureFile(&a.flowFile, "flow")
	default:
		return nil
	}
}

// WriteTraceComplete persists a TRACE-01 completion row into trace-*.jsonl.
func (a *FlowFileAppender) WriteTraceComplete(row map[string]any) {
	if a == nil || row == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.writeMutedLocked() {
		return
	}
	target := a.ensureFile(&a.traceFile, "trace")
	if target != nil {
		a.writeRowLocked(target, row)
	}
}

// writeMutedLocked reports whether file writes are currently muted by the
// circuit breaker. When the mute window has expired it re-arms the breaker
// (half-open probe: the next open/write is attempted and re-trips on
// failure).
func (a *FlowFileAppender) writeMutedLocked() bool {
	if a.writeMutedUntil.IsZero() {
		return false
	}
	if time.Now().Before(a.writeMutedUntil) {
		return true
	}
	a.writeMutedUntil = time.Time{}
	a.writeFailStreak = 0
	return false
}

// noteFailureLocked records one open/write failure and trips the circuit
// breaker after flowFileWriteFailThreshold consecutive failures, reporting
// whether it just tripped.
func (a *FlowFileAppender) noteFailureLocked() (tripped bool) {
	a.writeFailStreak++
	if a.writeFailStreak >= flowFileWriteFailThreshold {
		a.writeFailStreak = 0
		a.writeMutedUntil = time.Now().Add(flowFileWriteMuteDuration)
		return true
	}
	return false
}

// warnMutedLocked emits the single "writes muted" Warn when the breaker trips.
func (a *FlowFileAppender) warnMutedLocked(path string, err error) {
	a.lg.Warn("FlowFileAppender: writes muted after repeated failures",
		loggateway.StepID("monitor.flow_file.write_muted"),
		loggateway.Str("path", path),
		loggateway.Str("mute_duration", flowFileWriteMuteDuration.String()),
		loggateway.Err(err))
}

func (a *FlowFileAppender) ensureFile(slot **rotatingFile, prefix string) *rotatingFile {
	today := time.Now().UTC().Format("2006-01-02")
	// Fast path: same date and no rotation needed.
	if *slot != nil && (*slot).date == today && !(*slot).shouldRotate() {
		return *slot
	}
	// Capture rotation decision BEFORE Close(): shouldRotate() returns false
	// once rf.file is nil (set by Close), so checking after Close would never
	// trigger size-based rotation — causing unbounded growth of a single file.
	seq := 1
	if *slot != nil {
		if (*slot).date == today && (*slot).shouldRotate() {
			seq = (*slot).seq + 1
		}
		(*slot).Close()
	}
	baseName := fmt.Sprintf("%s-%s.jsonl", prefix, today)
	path := baseName
	if seq > 1 {
		path = fmt.Sprintf("%s.%d", baseName, seq)
	}
	fullPath := filepath.Join(a.dir, path)
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		// P1: open failures feed the same circuit breaker as write failures.
		if a.noteFailureLocked() {
			a.warnMutedLocked(fullPath, err)
			return nil
		}
		a.lg.Warn("FlowFileAppender: open failed", loggateway.StepID("monitor.flow_file.open_fail"), loggateway.Str("path", fullPath), loggateway.Err(err))
		return nil
	}
	rf := &rotatingFile{
		path:     fullPath,
		file:     f,
		encoder:  json.NewEncoder(f),
		date:     today,
		maxSize:  500 * 1024 * 1024,
		seq:      seq,
		baseName: baseName,
	}
	*slot = rf
	return rf
}

func (a *FlowFileAppender) writeRowLocked(rf *rotatingFile, row map[string]any) {
	if rf == nil {
		return
	}
	// The circuit breaker mute check lives at the entry points
	// (onMonitorEvent / WriteTraceComplete); here we only count failures.
	if err := rf.encoder.Encode(row); err != nil {
		if a.noteFailureLocked() {
			a.warnMutedLocked(rf.path, err)
			return
		}
		a.lg.Warn("FlowFileAppender: write failed", loggateway.StepID("monitor.flow_file.write_fail"), loggateway.Str("path", rf.path), loggateway.Err(err))
		return
	}
	a.writeFailStreak = 0
}

// Dir returns the flow file directory path.
func (a *FlowFileAppender) Dir() string {
	if a == nil {
		return ""
	}
	return a.dir
}

// PurgeExpiredFiles removes expired flow log files and returns the count purged.
func (a *FlowFileAppender) PurgeExpiredFiles() int {
	return a.purgeExpiredFiles()
}

// CompressOldFiles compresses old flow log files and returns the count compressed.
func (a *FlowFileAppender) CompressOldFiles() int {
	return a.compressOldFiles()
}

// PurgeExcessBackups removes rotated backup files beyond the maxBackups cap
// and returns the count purged.
func (a *FlowFileAppender) PurgeExcessBackups() int {
	return a.purgeExcessBackups()
}

func (rf *rotatingFile) Close() {
	if rf != nil && rf.file != nil {
		rf.file.Sync()
		rf.file.Close()
		rf.file = nil
	}
}
