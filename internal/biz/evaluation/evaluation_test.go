package evaluation

import "testing"

func TestParseScores(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want Scores
	}{
		{"empty string", "", Scores{}},
		{"empty json object", "{}", Scores{}},
		{"valid scores", `{"exact_match":0.9,"relevance":0.8}`, Scores{"exact_match": 0.9, "relevance": 0.8}},
		{"invalid json", "not-json", Scores{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseScores(tt.raw)
			if len(got) != len(tt.want) {
				t.Fatalf("ParseScores(%q) returned %d keys, want %d", tt.raw, len(got), len(tt.want))
			}
			for k, v := range tt.want {
				gv, ok := got[k]
				if !ok {
					t.Errorf("ParseScores(%q) missing key %q", tt.raw, k)
				}
				if gv != v {
					t.Errorf("ParseScores(%q)[%q] = %v, want %v", tt.raw, k, gv, v)
				}
			}
		})
	}
}

func TestMarshalScores(t *testing.T) {
	tests := []struct {
		name string
		m    Scores
		want string
	}{
		{"nil map", nil, "{}"},
		{"empty map", Scores{}, "{}"},
		{"map with values", Scores{"exact_match": 0.9, "relevance": 0.8}, `{"exact_match":0.9,"relevance":0.8}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MarshalScores(tt.m)
			if got != tt.want {
				t.Errorf("MarshalScores() = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("roundtrip ParseScores MarshalScores", func(t *testing.T) {
		original := Scores{"exact_match": 0.9, "relevance": 0.8}
		roundtrip := ParseScores(MarshalScores(original))
		if len(roundtrip) != len(original) {
			t.Fatalf("roundtrip length = %d, want %d", len(roundtrip), len(original))
		}
		for k, v := range original {
			gv, ok := roundtrip[k]
			if !ok {
				t.Errorf("roundtrip missing key %q", k)
			}
			if gv != v {
				t.Errorf("roundtrip[%q] = %v, want %v", k, gv, v)
			}
		}
	})
}

func TestLLMSetting_SimConfigured(t *testing.T) {
	tests := []struct {
		name    string
		setting LLMSetting
		want    bool
	}{
		{"empty returns false", LLMSetting{}, false},
		{"provider only returns false", LLMSetting{SimProvider: "openai"}, false},
		{"model only returns false", LLMSetting{SimModel: "gpt-4"}, false},
		{"both set returns true", LLMSetting{SimProvider: "openai", SimModel: "gpt-4"}, true},
		{"whitespace only returns false", LLMSetting{SimProvider: "  ", SimModel: "  "}, false},
		{"provider whitespace model set returns false", LLMSetting{SimProvider: "  ", SimModel: "gpt-4"}, false},
		{"provider set model whitespace returns false", LLMSetting{SimProvider: "openai", SimModel: "  "}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.setting.SimConfigured()
			if got != tt.want {
				t.Errorf("SimConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLLMSetting_JudgeConfigured(t *testing.T) {
	tests := []struct {
		name    string
		setting LLMSetting
		want    bool
	}{
		{"empty returns false", LLMSetting{}, false},
		{"provider only returns false", LLMSetting{JudgeProvider: "openai"}, false},
		{"model only returns false", LLMSetting{JudgeModel: "gpt-4"}, false},
		{"both set returns true", LLMSetting{JudgeProvider: "openai", JudgeModel: "gpt-4"}, true},
		{"whitespace only returns false", LLMSetting{JudgeProvider: "  ", JudgeModel: "  "}, false},
		{"provider whitespace model set returns false", LLMSetting{JudgeProvider: "  ", JudgeModel: "gpt-4"}, false},
		{"provider set model whitespace returns false", LLMSetting{JudgeProvider: "openai", JudgeModel: "  "}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.setting.JudgeConfigured()
			if got != tt.want {
				t.Errorf("JudgeConfigured() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestApplyLLMPatch(t *testing.T) {
	tests := []struct {
		name          string
		cur           LLMSetting
		simProvider   string
		simModel      string
		judgeProvider string
		judgeModel    string
		want          LLMSetting
	}{
		{
			"empty patch preserves current",
			LLMSetting{SimProvider: "a", SimModel: "b", JudgeProvider: "c", JudgeModel: "d"},
			"", "", "", "",
			LLMSetting{SimProvider: "a", SimModel: "b", JudgeProvider: "c", JudgeModel: "d"},
		},
		{
			"sim only patch",
			LLMSetting{SimProvider: "old", SimModel: "old", JudgeProvider: "jp", JudgeModel: "jm"},
			"new-p", "new-m", "", "",
			LLMSetting{SimProvider: "new-p", SimModel: "new-m", JudgeProvider: "jp", JudgeModel: "jm"},
		},
		{
			"judge only patch",
			LLMSetting{SimProvider: "sp", SimModel: "sm", JudgeProvider: "old", JudgeModel: "old"},
			"", "", "new-jp", "new-jm",
			LLMSetting{SimProvider: "sp", SimModel: "sm", JudgeProvider: "new-jp", JudgeModel: "new-jm"},
		},
		{
			"full patch",
			LLMSetting{},
			"sp", "sm", "jp", "jm",
			LLMSetting{SimProvider: "sp", SimModel: "sm", JudgeProvider: "jp", JudgeModel: "jm"},
		},
		{
			"partial patch preserves existing values",
			LLMSetting{SimProvider: "sp", SimModel: "sm", JudgeProvider: "jp", JudgeModel: "jm"},
			"sp2", "", "", "",
			LLMSetting{SimProvider: "sp2", SimModel: "", JudgeProvider: "jp", JudgeModel: "jm"},
		},
		{
			"patch trims whitespace",
			LLMSetting{},
			"  sp  ", "  sm  ", "  jp  ", "  jm  ",
			LLMSetting{SimProvider: "sp", SimModel: "sm", JudgeProvider: "jp", JudgeModel: "jm"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyLLMPatch(tt.cur, tt.simProvider, tt.simModel, tt.judgeProvider, tt.judgeModel)
			if got != tt.want {
				t.Errorf("ApplyLLMPatch() = %+v, want %+v", got, tt.want)
			}
		})
	}
}
