package loggateway

import "time"

type Step struct {
	g      *Gateway
	stepID string
	start  time.Time
}

func (s *Step) Done(msg string, fields ...Field) {
	elapsed := time.Since(s.start).Milliseconds()
	all := make([]Field, 0, len(fields)+3)
	all = append(all, StepID(s.stepID), Phase("done"), Duration(elapsed))
	all = append(all, fields...)
	s.g.Info(msg, all...)
}

func (s *Step) Warn(msg string, fields ...Field) {
	elapsed := time.Since(s.start).Milliseconds()
	all := make([]Field, 0, len(fields)+3)
	all = append(all, StepID(s.stepID), Phase("warn"), Duration(elapsed))
	all = append(all, fields...)
	s.g.Warn(msg, all...)
}

func (s *Step) Error(msg string, fields ...Field) {
	elapsed := time.Since(s.start).Milliseconds()
	all := make([]Field, 0, len(fields)+3)
	all = append(all, StepID(s.stepID), Phase("error"), Duration(elapsed))
	all = append(all, fields...)
	s.g.Error(msg, all...)
}
