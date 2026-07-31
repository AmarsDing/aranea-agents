package data

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ── File-based test-run reader (73-self-iteration-v3, T1.9) ─────────────────
//
// TestFailureTrigger 的信号源：外部 cron（全量测试任务）每轮把失败清单写为
// 一个 JSON 文件放入目录，本 reader 聚合连续轮次失败。
//
// 文件格式（每轮一个文件，文件名任意，按内容 round 时间排序）：
//
//	{"round":"2026-07-29T10:00:00Z","failures":[
//	  {"package":"./internal/biz","test_name":"TestFoo","error":"assert failed"}]}
//
// 目录不存在/为空 = 无信号（返回空，不报错）；坏文件跳过。
// 最多回溯 siTestRunMaxRounds 个最近轮次。

// siTestRunMaxRounds caps how many recent rounds are scanned.
const siTestRunMaxRounds = 20

// TestRunFileReader implements biz.TestRunReader over a JSON-file directory.
type TestRunFileReader struct {
	dir string
}

var _ biz.TestRunReader = (*TestRunFileReader)(nil)

// NewTestRunFileReader creates the reader rooted at dir (config:
// self_improvement.test_runs_dir；默认 test/test-runs)。
func NewTestRunFileReader(dir string) *TestRunFileReader {
	return &TestRunFileReader{dir: dir}
}

type siTestRunFile struct {
	Round    string `json:"round"`
	Failures []struct {
		Package  string `json:"package"`
		TestName string `json:"test_name"`
		Error    string `json:"error"`
	} `json:"failures"`
}

// ListRecentFailures implements biz.TestRunReader.
func (r *TestRunFileReader) ListRecentFailures(_ context.Context, minConsecutiveRounds int) ([]biz.TestFailure, error) {
	if r == nil || r.dir == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // cron 尚未产出 → 无信号
		}
		return nil, apierror.Internal("SELF_IMPROVE", "read test-runs dir: "+err.Error())
	}

	// 解析全部轮次文件（坏文件跳过），按 round 时间从新到旧排序。
	type round struct {
		at       time.Time
		failures map[string]biz.TestFailure // key: package\x00test
	}
	var rounds []round
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			continue
		}
		var f siTestRunFile
		if err := json.Unmarshal(raw, &f); err != nil {
			continue
		}
		at := parseSITime(f.Round)
		if at.IsZero() {
			continue
		}
		rd := round{at: at, failures: map[string]biz.TestFailure{}}
		for _, fl := range f.Failures {
			key := fl.Package + "\x00" + fl.TestName
			rd.failures[key] = biz.TestFailure{
				Package:   fl.Package,
				TestName:  fl.TestName,
				LastError: fl.Error,
				LastSeen:  at,
			}
		}
		rounds = append(rounds, rd)
	}
	if len(rounds) == 0 {
		return nil, nil
	}
	sort.Slice(rounds, func(i, j int) bool { return rounds[i].at.After(rounds[j].at) })
	if len(rounds) > siTestRunMaxRounds {
		rounds = rounds[:siTestRunMaxRounds]
	}

	// 连续失败统计：从最新轮起，同一测试在每轮都出现则计数累加，
	// 遇到缺席轮即停止；最新轮缺席的测试视为已恢复，不上报。
	if minConsecutiveRounds <= 0 {
		minConsecutiveRounds = 1
	}
	latest := rounds[0]
	var out []biz.TestFailure
	for key, f := range latest.failures {
		consecutive := 1
		for i := 1; i < len(rounds); i++ {
			if _, ok := rounds[i].failures[key]; !ok {
				break
			}
			consecutive++
		}
		if consecutive < minConsecutiveRounds {
			continue
		}
		f.ConsecutiveRounds = consecutive
		out = append(out, f)
	}
	// 稳定排序（连续轮次降序、包名、测试名），保证多次扫描输出一致。
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConsecutiveRounds != out[j].ConsecutiveRounds {
			return out[i].ConsecutiveRounds > out[j].ConsecutiveRounds
		}
		if out[i].Package != out[j].Package {
			return out[i].Package < out[j].Package
		}
		return out[i].TestName < out[j].TestName
	})
	return out, nil
}
