//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package dataset

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// LongMemEvalTurn represents one turn in a LongMemEval session.
type LongMemEvalTurn struct {
	Role      string `json:"role"`
	Content   string `json:"content"`
	HasAnswer bool   `json:"has_answer,omitempty"`
}

// LongMemEvalInstance represents one evaluation instance from LongMemEval.
type LongMemEvalInstance struct {
	QuestionID       string              `json:"question_id"`
	QuestionType     string              `json:"question_type"`
	Question         string              `json:"question"`
	QuestionDate     string              `json:"question_date"`
	Answer           string              `json:"-"`
	RawAnswer        json.RawMessage     `json:"answer"`
	AnswerSessionIDs []string            `json:"answer_session_ids"`
	HaystackDates    []string            `json:"haystack_dates"`
	HaystackSessIDs  []string            `json:"haystack_session_ids"`
	HaystackSessions [][]LongMemEvalTurn `json:"haystack_sessions"`
}

// TotalTurns returns the total number of turns across all haystack sessions.
func (inst *LongMemEvalInstance) TotalTurns() int {
	total := 0
	for _, sess := range inst.HaystackSessions {
		total += len(sess)
	}
	return total
}

// EvidenceTurnCount returns the number of turns marked has_answer=true.
func (inst *LongMemEvalInstance) EvidenceTurnCount() int {
	count := 0
	for _, sess := range inst.HaystackSessions {
		for _, turn := range sess {
			if turn.HasAnswer {
				count++
			}
		}
	}
	return count
}

// LoadLongMemEval loads LongMemEval instances from a JSON file.
func LoadLongMemEval(path string) ([]*LongMemEvalInstance, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read longmemeval file: %w", err)
	}

	var instances []*LongMemEvalInstance
	if err := json.Unmarshal(data, &instances); err != nil {
		return nil, fmt.Errorf("parse longmemeval JSON: %w", err)
	}

	// Post-process: parse RawAnswer into Answer string (handles int/string)
	for _, inst := range instances {
		if len(inst.RawAnswer) == 0 {
			continue
		}
		var s string
		if err := json.Unmarshal(inst.RawAnswer, &s); err == nil {
			inst.Answer = s
		} else {
			// Fallback: use raw value as string (e.g., numbers)
			inst.Answer = strings.TrimSpace(string(inst.RawAnswer))
		}
	}
	return instances, nil
}

// FilterLongMemEval filters instances by question type.
// If questionTypes is empty, all instances are returned.
func FilterLongMemEval(
	instances []*LongMemEvalInstance,
	questionTypes []string,
) []*LongMemEvalInstance {
	if len(questionTypes) == 0 {
		return instances
	}

	typeSet := make(map[string]struct{}, len(questionTypes))
	for _, qt := range questionTypes {
		typeSet[strings.TrimSpace(strings.ToLower(qt))] = struct{}{}
	}

	var filtered []*LongMemEvalInstance
	for _, inst := range instances {
		normalized := strings.TrimSpace(strings.ToLower(inst.QuestionType))
		if _, ok := typeSet[normalized]; ok {
			filtered = append(filtered, inst)
		}
	}

	sort.SliceStable(filtered, func(i, j int) bool {
		return filtered[i].QuestionID < filtered[j].QuestionID
	})
	return filtered
}
