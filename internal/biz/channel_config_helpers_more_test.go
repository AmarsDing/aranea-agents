package biz

import "testing"

func TestNormalizeAsyncKeywords(t *testing.T) {
	cases := []struct {
		input []string
		want  []string
	}{
		{[]string{}, []string{}},
		{[]string{"a", "b"}, []string{"a", "b"}},
		{[]string{"  a  ", "b", "  "}, []string{"a", "b"}},
		{[]string{"", "a", ""}, []string{"a"}},
	}
	for _, tc := range cases {
		got := normalizeAsyncKeywords(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("normalizeAsyncKeywords(%v) = %v, want %v", tc.input, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("normalizeAsyncKeywords(%v)[%d] = %q, want %q", tc.input, i, got[i], tc.want[i])
			}
		}
	}
}

func TestMatchesChannelAsyncKeyword(t *testing.T) {
	cases := []struct {
		text     string
		keywords []string
		want     bool
	}{
		{"请做全量分析", []string{"全量", "研报"}, true},
		{"写一份研报", []string{"全量", "研报"}, true},
		{"今天天气怎么样", []string{"全量", "研报"}, false},
		{"/async help", []string{"/async"}, true},
		{"/ASYNC help", []string{"/async"}, true},
		{"hello /async", []string{"/async"}, false},
		{"", []string{"全量"}, false},
		{"分析", []string{}, false},
		{"hello", nil, false},
	}
	for _, tc := range cases {
		got := matchesChannelAsyncKeyword(tc.text, tc.keywords)
		if got != tc.want {
			t.Errorf("matchesChannelAsyncKeyword(%q, %v) = %v, want %v", tc.text, tc.keywords, got, tc.want)
		}
	}
}

func TestChannelLongTaskConfig_AsyncKeywords(t *testing.T) {
	cfg := ChannelLongTaskConfig{AsyncKeywords: []string{"custom1", "custom2"}}
	got := cfg.asyncKeywords()
	if len(got) != 2 || got[0] != "custom1" {
		t.Fatalf("asyncKeywords() = %v", got)
	}

	cfgEmpty := ChannelLongTaskConfig{}
	gotDefault := cfgEmpty.asyncKeywords()
	if len(gotDefault) != len(DefaultChannelAsyncKeywords) {
		t.Fatalf("asyncKeywords() default = %v", gotDefault)
	}
}

func TestChannelLongTaskConfig_HasAsyncTarget(t *testing.T) {
	cases := []struct {
		cfg  ChannelLongTaskConfig
		want bool
	}{
		{ChannelLongTaskConfig{}, false},
		{ChannelLongTaskConfig{AsyncGraphID: "g1"}, true},
		{ChannelLongTaskConfig{AsyncTeamID: "t1"}, true},
		{ChannelLongTaskConfig{AsyncCronTaskID: "c1"}, true},
	}
	for _, tc := range cases {
		got := tc.cfg.hasAsyncTarget()
		if got != tc.want {
			t.Errorf("hasAsyncTarget() = %v, want %v", got, tc.want)
		}
	}
}

func TestChannelLongTaskConfig_MaxConcurrentInbound(t *testing.T) {
	cases := []struct {
		cfg     ChannelLongTaskConfig
		isGroup bool
		want    int
	}{
		{ChannelLongTaskConfig{}, false, defaultChannelMaxConcurrentDM},
		{ChannelLongTaskConfig{}, true, defaultChannelMaxConcurrentGroup},
		{ChannelLongTaskConfig{SessionMaxConcurrentDM: 3}, false, 3},
		{ChannelLongTaskConfig{SessionMaxConcurrentGroup: 5}, true, 5},
	}
	for _, tc := range cases {
		got := tc.cfg.MaxConcurrentInbound(tc.isGroup)
		if got != tc.want {
			t.Errorf("MaxConcurrentInbound(isGroup=%v) = %d, want %d", tc.isGroup, got, tc.want)
		}
	}
}

func TestChannelLongTaskConfig_ProgressEnabled(t *testing.T) {
	cases := []struct {
		mode string
		want bool
	}{
		{"", false},
		{"off", false},
		{"OFF", false},
		{"text", true},
		{"steps", true},
	}
	for _, tc := range cases {
		cfg := ChannelLongTaskConfig{ProgressMode: tc.mode}
		got := cfg.ProgressEnabled()
		if got != tc.want {
			t.Errorf("ProgressEnabled(mode=%q) = %v, want %v", tc.mode, got, tc.want)
		}
	}
}

func TestChannelBusyInputInterrupt(t *testing.T) {
	cfg := `{"config":{"busy_input_mode":"interrupt"}}`
	if !ChannelBusyInputInterrupt(cfg) {
		t.Fatal("expected interrupt=true")
	}
	if ChannelBusyInputFollowup(cfg) {
		t.Fatal("interrupt should not be followup")
	}
}

func TestChannelBusyInputFollowup(t *testing.T) {
	cfg := `{"config":{"busy_input_mode":"followup"}}`
	if !ChannelBusyInputFollowup(cfg) {
		t.Fatal("expected followup=true")
	}
	if ChannelBusyInputInterrupt(cfg) {
		t.Fatal("followup should not be interrupt")
	}
}

func TestChannelBusyInputQueue(t *testing.T) {
	cases := []struct {
		configJSON string
		want       bool
	}{
		{`{"config":{}}`, true},
		{`{"config":{"busy_input_mode":"queue"}}`, true},
		{`{"config":{"busy_input_mode":"followup"}}`, true},
		{`{"config":{"busy_input_mode":"interrupt"}}`, false},
	}
	for _, tc := range cases {
		got := ChannelBusyInputQueue(tc.configJSON)
		if got != tc.want {
			t.Errorf("ChannelBusyInputQueue(%q) = %v, want %v", tc.configJSON, got, tc.want)
		}
	}
}

func TestChannelLongTaskConfig_SuggestDurableRun(t *testing.T) {
	cfg := ChannelLongTaskConfig{AsyncKeywords: []string{"全量", "研报"}}
	cases := []struct {
		text string
		want bool
	}{
		{"", false},
		{"/async help", true},
		{"/background", true},
		{"请做全量分析报告", true},
		{"分析", false},
		{"短文本", false},
	}
	for _, tc := range cases {
		got := cfg.SuggestDurableRun(tc.text)
		if got != tc.want {
			t.Errorf("SuggestDurableRun(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}

func TestParseWeChatActiveMode(t *testing.T) {
	cases := []struct {
		configJSON string
		want       bool
	}{
		{`{"config":{"active_mode":true}}`, true},
		{`{"config":{"active_mode":false}}`, false},
		{`{"config":{}}`, false},
		{`{}`, false},
	}
	for _, tc := range cases {
		got := ParseWeChatActiveMode(tc.configJSON)
		if got != tc.want {
			t.Errorf("ParseWeChatActiveMode(%q) = %v, want %v", tc.configJSON, got, tc.want)
		}
	}
}

func TestChannelStreamingEnabled(t *testing.T) {
	cases := []struct {
		configJSON string
		want       bool
	}{
		{`{"config":{"streaming_enabled":true}}`, true},
		{`{"config":{"streaming_enabled":false}}`, false},
		{`{"config":{}}`, false},
		{`{}`, false},
	}
	for _, tc := range cases {
		got := ChannelStreamingEnabled(tc.configJSON)
		if got != tc.want {
			t.Errorf("ChannelStreamingEnabled(%q) = %v, want %v", tc.configJSON, got, tc.want)
		}
	}
}
