package registry

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// Registry 是已加载工作流的内存注册表。热加载经 Swap 原子替换;Get/List 读锁并发安全。
type Registry struct {
	mu        sync.RWMutex
	Workflows map[string]*Workflow
	Order     []string // 按文件名字典序,保证装配确定性
}

// Get 按 id 取工作流。
func (r *Registry) Get(id string) *Workflow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Workflows[id]
}

// List 按确定顺序返回全部工作流。
func (r *Registry) List() []*Workflow {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Workflow, 0, len(r.Order))
	for _, id := range r.Order {
		out = append(out, r.Workflows[id])
	}
	return out
}

// Swap 原子替换注册表内容(热加载成功路径;调用方保证 newReg 完整可用)。
func (r *Registry) Swap(newReg *Registry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Workflows = newReg.Workflows
	r.Order = newReg.Order
}

// Reload 重新加载目录:全部声明成功才 Swap;任何一条失败(或目录不可读)整体回退,
// 旧版本继续服务,返回报告与错误。防"改坏一条声明 → 看板整页掉工作流"。
func (r *Registry) Reload(dir string) (*Report, error) {
	newReg, rep, err := LoadDir(dir)
	if err != nil {
		return nil, err
	}
	if len(rep.Failed) > 0 {
		return rep, fmt.Errorf("%d 条声明加载失败,热加载整体回退", len(rep.Failed))
	}
	r.Swap(newReg)
	return rep, nil
}

// ReportHolder 并发安全地持有"最近一次加载报告"(热加载后替换)。
type ReportHolder struct {
	mu  sync.RWMutex
	rep *Report
}

// NewReportHolder 构造。
func NewReportHolder(rep *Report) *ReportHolder { return &ReportHolder{rep: rep} }

// Get 取当前报告。
func (h *ReportHolder) Get() *Report {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.rep
}

// Set 替换当前报告。
func (h *ReportHolder) Set(rep *Report) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.rep = rep
}

// LoadDir 加载目录下全部 *.yaml/yml 声明。
// 行为(§16.2):单条声明解析/校验失败 → 该工作流不加载并记入 Report.Failed,其余正常;
// 含 [待补] 标记的元素显式跳过并记入 Report.Skipped(不臆造字段)。
func LoadDir(dir string) (*Registry, *Report, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("读注册表目录失败: %w", err)
	}
	reg := &Registry{Workflows: map[string]*Workflow{}, Order: []string{}}
	rep := &Report{Dir: dir, LoadedAt: time.Now(), Loaded: []string{}, Failed: []FailEntry{}, Skipped: []SkipEntry{}}

	var files []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || (!strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml")) {
			continue
		}
		files = append(files, name)
	}
	sort.Strings(files)

	for _, name := range files {
		full := filepath.Join(dir, name)
		raw, err := os.ReadFile(full)
		if err != nil {
			rep.Failed = append(rep.Failed, FailEntry{File: name, Errors: []string{err.Error()}})
			continue
		}
		var wf Workflow
		if err := yaml.Unmarshal(raw, &wf); err != nil {
			rep.Failed = append(rep.Failed, FailEntry{File: name, Errors: []string{"YAML 解析失败: " + err.Error()}})
			continue // 隔离失败:不影响其他文件
		}
		wf.File = name
		if errs := Validate(&wf); len(errs) > 0 {
			rep.Failed = append(rep.Failed, FailEntry{File: name, Errors: errs})
			continue
		}
		if _, dup := reg.Workflows[wf.ID]; dup {
			rep.Failed = append(rep.Failed, FailEntry{File: name, Errors: []string{"工作流 id 重复: " + wf.ID}})
			continue
		}
		applyPending(&wf, rep)
		reg.Workflows[wf.ID] = &wf
		reg.Order = append(reg.Order, wf.ID)
		rep.Loaded = append(rep.Loaded, wf.ID)
	}
	return reg, rep, nil
}

// applyPending 扫描声明中的 [待补] 标记,显式跳过对应元素并记录。
// 跳过粒度:source.rest_path 含标记 → 整个 source 跳过;field_map 单条值含标记 → 跳过该映射;
// metric.pending 非空 → 该指标跳过(保留占位供前端显示"数据未接通")。
func applyPending(wf *Workflow, rep *Report) {
	for i := range wf.Nodes {
		n := &wf.Nodes[i]
		if n.Source != nil {
			s := n.Source
			if strings.Contains(s.RestPath, PendingMarker) {
				rep.Skipped = append(rep.Skipped, SkipEntry{
					Workflow: wf.ID, Node: n.ID, Field: "source.rest_path", Reason: s.RestPath,
				})
				s.Skipped = true
				s.SkipReason = s.RestPath
				s.RestPath = ""
			}
			for k, v := range s.FieldMap {
				if strings.Contains(v, PendingMarker) {
					rep.Skipped = append(rep.Skipped, SkipEntry{
						Workflow: wf.ID, Node: n.ID, Field: "field_map." + k, Reason: v,
					})
					delete(s.FieldMap, k)
				}
			}
		}
		for j := range n.Metrics {
			m := &n.Metrics[j]
			if m.Pending != "" {
				rep.Skipped = append(rep.Skipped, SkipEntry{
					Workflow: wf.ID, Node: n.ID, Metric: m.ID, Field: "metric.pending", Reason: m.Pending,
				})
			}
		}
	}
}
