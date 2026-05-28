package monitor

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/safego"
)

type FlowFileAppender struct {
	dir           string
	retentionDays int
	compressAge   time.Duration

	mu         sync.Mutex
	flowFile   *rotatingFile
	systemFile *rotatingFile
	traceFile  *rotatingFile
	alertFile  *rotatingFile
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

func NewFlowFileAppender(dir string) *FlowFileAppender {
	if dir == "" {
		dir = "/var/log/aranea"
	}
	return &FlowFileAppender{
		dir:           dir,
		retentionDays: 30,
		compressAge:   24 * time.Hour,
	}
}

func (a *FlowFileAppender) Start(ctx context.Context, buses ...contract.Bus) {
	if a == nil {
		return
	}
	if err := os.MkdirAll(a.dir, 0755); err != nil {
		event.SysLogWarn("system.monitor.flow_file.mkdir_fail", "FlowFileAppender: mkdir failed", event.P("dir", a.dir), event.P("error", err.Error()))
		return
	}
	opts := contract.SubscribeOptions{
		EventTypes: []contract.EnvelopeType{
			contract.EnvelopeTypeFlowLog,
			contract.EnvelopeTypeAlertNotify,
			contract.EnvelopeTypeMCPHealthAlert,
			contract.EnvelopeTypeRunnerCompletion,
		},
		BufferSize: 1024,
		DropPolicy: contract.DropNewest,
	}
	for i, bus := range buses {
		if bus == nil {
			continue
		}
		name := "flow-file-appender"
		if len(buses) > 1 {
			name = fmt.Sprintf("flow-file-appender-%d", i)
		}
		ch, unsub := bus.Subscribe(opts)
		safego.Go(ctx, name, func() {
			defer unsub()
			for {
				select {
				case <-ctx.Done():
					return
				case env, ok := <-ch:
					if !ok {
						return
					}
					a.onEnvelope(env)
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
	compressed := a.compressOldFiles()
	purged := a.purgeExpiredFiles()
	a.purgeTmpFiles()
	if compressed > 0 || purged > 0 {
		event.SysLogInfo("system.monitor.flow_file.maintenance", "FlowFileAppender maintenance completed",
			event.P("compressed", fmt.Sprint(compressed)), event.P("purged", fmt.Sprint(purged)))
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
		if !strings.HasSuffix(name, ".jsonl") && !strings.HasSuffix(name, ".jsonl.gz") {
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

func (a *FlowFileAppender) onEnvelope(env contract.Envelope) {
	if a == nil || env.Metadata == nil {
		return
	}

	row := make(map[string]any, len(env.Metadata)+4)
	for k, v := range env.Metadata {
		row[k] = v
	}
	row["_ts"] = env.Timestamp
	row["_id"] = env.ID
	row["_session_id"] = env.SessionID
	if env.Content != nil {
		row["_text"] = env.Content.Text
	}

	target := a.routeFile(env)
	if target == nil {
		return
	}

	a.writeRow(target, row)
}

func (a *FlowFileAppender) routeFile(env contract.Envelope) *rotatingFile {
	a.mu.Lock()
	defer a.mu.Unlock()

	switch env.Type {
	case contract.EnvelopeTypeAlertNotify, contract.EnvelopeTypeMCPHealthAlert:
		return a.ensureFile(&a.alertFile, "alert")
	case contract.EnvelopeTypeRunnerCompletion:
		return a.ensureFile(&a.traceFile, "trace")
	case contract.EnvelopeTypeFlowLog:
		ch := strings.TrimSpace(env.Channel)
		if ch == "monitor" {
			return a.ensureFile(&a.systemFile, "system")
		}
		return a.ensureFile(&a.flowFile, "flow")
	default:
		return nil
	}
}

func (a *FlowFileAppender) ensureFile(slot **rotatingFile, prefix string) *rotatingFile {
	today := time.Now().UTC().Format("2006-01-02")
	if *slot != nil && (*slot).date == today && !(*slot).shouldRotate() {
		return *slot
	}
	if *slot != nil {
		(*slot).Close()
	}
	baseName := fmt.Sprintf("%s-%s.jsonl", prefix, today)
	seq := 1
	if *slot != nil && (*slot).date == today && (*slot).shouldRotate() {
		seq = (*slot).seq + 1
	}
	path := baseName
	if seq > 1 {
		path = fmt.Sprintf("%s.%d", baseName, seq)
	}
	fullPath := filepath.Join(a.dir, path)
	f, err := os.OpenFile(fullPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		event.SysLogWarn("system.monitor.flow_file.open_fail", "FlowFileAppender: open failed", event.P("path", fullPath), event.P("error", err.Error()))
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

func (a *FlowFileAppender) writeRow(rf *rotatingFile, row map[string]any) {
	if rf == nil {
		return
	}
	if err := rf.encoder.Encode(row); err != nil {
		event.SysLogWarn("system.monitor.flow_file.write_fail", "FlowFileAppender: write failed", event.P("path", rf.path), event.P("error", err.Error()))
		return
	}
}

func (rf *rotatingFile) Close() {
	if rf != nil && rf.file != nil {
		rf.file.Sync()
		rf.file.Close()
		rf.file = nil
	}
}
