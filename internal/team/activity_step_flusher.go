package team

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/conf"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// ActivityStepFlusher asynchronously persists orchestration activity rows.
type ActivityStepFlusher struct {
	repo             biz.OrchestrationStepRepo
	runID            string
	graphExecutionID string
	ch               chan biz.OrchestrationStep
	stop             chan struct{}
	done             chan struct{}
	once             sync.Once
	flushConf        conf.RuntimeActivityFlusherConfig
	lg               loggateway.Logger
}

// NewActivityStepFlusher creates an ActivityStepFlusher. // WIRE: needs *conf.Runtime
func NewActivityStepFlusher(runtimeConf *conf.Runtime, repo biz.OrchestrationStepRepo, runID, graphExecutionID string, lg loggateway.Logger) *ActivityStepFlusher {
	if repo == nil || strings.TrimSpace(runID) == "" || !obsPersistEnabled() {
		return nil
	}
	fc := runtimeConf.ActivityFlusherConfig()
	f := &ActivityStepFlusher{
		repo:             repo,
		runID:            runID,
		graphExecutionID: strings.TrimSpace(graphExecutionID),
		ch:               make(chan biz.OrchestrationStep, fc.ChannelBuffer),
		stop:             make(chan struct{}),
		done:             make(chan struct{}),
		flushConf:        fc,
		lg:               lg,
	}
	safego.Go(appctx.Ctx(), "orchestration.activity.flusher", f.loop)
	return f
}

func obsPersistEnabled() bool {
	v := strings.TrimSpace(os.Getenv("ARANEA_OBS_PERSIST"))
	if v == "" {
		return true
	}
	return v == "1" || strings.EqualFold(v, "true") || strings.EqualFold(v, "yes")
}

func (f *ActivityStepFlusher) Enqueue(nodeID string, snap biz.ActivitySnapshot) {
	if f == nil {
		return
	}
	raw, err := json.Marshal(snap)
	if err != nil {
		return
	}
	step := biz.OrchestrationStep{
		ID:                   uuid.NewString(),
		TeamRunID:            f.runID,
		GraphExecutionID:     f.graphExecutionID,
		NodeID:               strings.TrimSpace(nodeID),
		ActivitySnapshotJSON: string(raw),
		Status:               strings.TrimSpace(snap.Status),
		StartedAt:            strings.TrimSpace(snap.StartedAt),
		FinishedAt:           strings.TrimSpace(snap.FinishedAt),
		CreatedAt:            time.Now().UTC().Format(time.RFC3339),
	}
	select {
	case f.ch <- step:
	default:
	}
}

func (f *ActivityStepFlusher) Stop() {
	if f == nil {
		return
	}
	f.once.Do(func() {
		close(f.stop)
		<-f.done
	})
}

func (f *ActivityStepFlusher) loop() {
	defer close(f.done)
	ticker := time.NewTicker(f.flushConf.FlushInterval)
	defer ticker.Stop()
	pending := make([]biz.OrchestrationStep, 0, f.flushConf.BatchSize)
	flush := func() {
		if len(pending) == 0 {
			return
		}
		batch := append([]biz.OrchestrationStep(nil), pending...)
		pending = pending[:0]
		ctx, cancel := context.WithTimeout(context.Background(), f.flushConf.DBTimeout)
		if berr := f.repo.BatchCreateOrchestrationSteps(ctx, batch); berr != nil {
			f.lg.Warn("BatchCreateOrchestrationSteps failed",
				loggateway.StepID("team.step.batch_fail"),
				loggateway.Int("batch_size", len(batch)),
				loggateway.Err(berr))
		}
		cancel()
	}
	for {
		select {
		case <-f.stop:
			for {
				select {
				case step := <-f.ch:
					pending = append(pending, step)
				default:
					flush()
					return
				}
			}
		case step := <-f.ch:
			pending = append(pending, step)
			if len(pending) >= int(f.flushConf.BatchSize) {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}
