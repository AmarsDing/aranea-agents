package adkruntime

import (
	"sync"
	"time"

	"google.golang.org/adk/plugin"
	"google.golang.org/adk/tool"
)

type skillUsageRecord struct {
	InvokeCount int
	Success     int
	Failure     int
	DurationMS  int
	LastTool    string
	LastStatus  string
	LastAt      time.Time
}

var skillUsageStats = struct {
	sync.Mutex
	started map[string]time.Time
	records map[string]*skillUsageRecord
}{
	started: map[string]time.Time{},
	records: map[string]*skillUsageRecord{},
}

func newSkillUsageTrackerPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "skill_usage_tracker",
		BeforeToolCallback: func(_ tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			if !isSkillTool(t.Name(), args) {
				return nil, nil
			}
			key := toolInvocationKey(t.Name(), args)
			skillUsageStats.Lock()
			skillUsageStats.started[key] = time.Now()
			skillUsageStats.Unlock()
			return nil, nil
		},
		AfterToolCallback: func(_ tool.Context, t tool.Tool, args, result map[string]any, err error) (map[string]any, error) {
			if !isSkillTool(t.Name(), args) {
				return nil, nil
			}
			recordSkillUsage(t.Name(), args, err == nil)
			return nil, nil
		},
		OnToolErrorCallback: func(_ tool.Context, t tool.Tool, args map[string]any, err error) (map[string]any, error) {
			if !isSkillTool(t.Name(), args) {
				return nil, nil
			}
			recordSkillUsage(t.Name(), args, false)
			return nil, nil
		},
	})
}
