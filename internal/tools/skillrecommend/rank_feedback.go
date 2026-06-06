package skillrecommend

import "time"

// RankFeedback captures the outcome of a ranking decision for later analysis.
// It records what the ranker predicted (score) vs. what actually happened
// (success), enabling future weight tuning and feedback loops.
type RankFeedback struct {
	SkillID       string    // the skill slug that was ranked
	RankScore     float64   // the score assigned by the ranker (0-1)
	ActualSuccess bool      // whether the skill invocation actually succeeded
	Timestamp     time.Time // when the feedback was recorded
}
