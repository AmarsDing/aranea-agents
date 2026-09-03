package biz

import (
	"strings"
	"testing"
)

// TestInspectDeliverables P3-1：规则版产出语义检查器。覆盖五类发现、优先级
// （error_dump > refusal > thin）、递归文本提取与无契约降级。
func TestInspectDeliverables(t *testing.T) {
	t.Parallel()
	contract := &MemberDeliverableContract{Entries: []MemberDeliverableEntry{
		{Topic: "report", Required: true},
		{Topic: "data", Required: true},
	}}

	cases := []struct {
		name       string
		deliv      map[string]any
		contract   *MemberDeliverableContract
		wantKinds  []string // 期望出现的 kind（无序）
		wantAbsent []string // 期望不出现的 kind
	}{
		{
			name: "契约 Required topic 缺失",
			deliv: map[string]any{
				"report": "这是一份足够长的调研报告正文，覆盖了所有要求的分析维度与结论。",
			},
			contract:  contract,
			wantKinds: []string{DeliverableFindingMissingTopic},
		},
		{
			name: "无契约跳过 missing 检查",
			deliv: map[string]any{
				"report": "这是一份足够长的调研报告正文，覆盖了所有要求的分析维度与结论。",
			},
			contract:   nil,
			wantAbsent: []string{DeliverableFindingMissingTopic},
		},
		{
			name:       "空内容",
			deliv:      map[string]any{"report": "   "},
			wantKinds: []string{DeliverableFindingEmptyContent},
		},
		{
			name:       "过薄内容",
			deliv:      map[string]any{"report": "完成了。"},
			wantKinds: []string{DeliverableFindingThinContent},
		},
		{
			name:       "拒绝语（中文）",
			deliv:      map[string]any{"report": "我无法完成这个任务，因为缺少必要的数据源访问权限。"},
			wantKinds: []string{DeliverableFindingRefusal},
		},
		{
			name:       "拒绝语（英文）",
			deliv:      map[string]any{"report": "I cannot assist with this request because it requires external systems."},
			wantKinds: []string{DeliverableFindingRefusal},
		},
		{
			name:       "正文深处出现拒绝词不误伤",
			deliv:      map[string]any{"report": strings.Repeat("正常的分析内容段落。", 30) + "有人问我 cannot 的含义，这里解释如下。"},
			wantAbsent: []string{DeliverableFindingRefusal, DeliverableFindingThinContent},
		},
		{
			name:       "错误转储（panic）",
			deliv:      map[string]any{"report": "执行结果：panic: runtime error: index out of range [3] with length 3"},
			wantKinds: []string{DeliverableFindingErrorDump},
		},
		{
			name:       "错误转储（工具失败）",
			deliv:      map[string]any{"report": "工具调用失败：web_search 返回 503，重试三次均失败。"},
			wantKinds: []string{DeliverableFindingErrorDump},
		},
		{
			name: "错误转储优先于过薄",
			deliv: map[string]any{
				"report": "panic: boom",
			},
			wantKinds:  []string{DeliverableFindingErrorDump},
			wantAbsent: []string{DeliverableFindingThinContent},
		},
		{
			name: "嵌套结构递归提取",
			deliv: map[string]any{
				"report": map[string]any{
					"summary": "",
					"items":   []any{"", ""},
				},
			},
			wantKinds: []string{DeliverableFindingEmptyContent},
		},
		{
			name: "健康产出无发现",
			deliv: map[string]any{
				"report": "本报告系统梳理了目标领域的三类主流方案，给出对比矩阵与选型建议，并附风险评估。",
				"data":   map[string]any{"rows": []any{"样本 A 的指标为 42，样本 B 的指标为 37，差异显著，置信区间与采样方法见附录说明。"}},
			},
			contract:   contract,
			wantAbsent: []string{DeliverableFindingMissingTopic, DeliverableFindingEmptyContent, DeliverableFindingThinContent, DeliverableFindingRefusal, DeliverableFindingErrorDump},
		},
		{
			name:       "空 map 无发现",
			deliv:      map[string]any{},
			wantAbsent: []string{DeliverableFindingEmptyContent, DeliverableFindingRefusal},
		},
		{
			// 标量兑底：数字内容不再是「提取为空」，应按真实文本长度判 thin。
			name:       "纯数字标量按 thin 而非 empty 判",
			deliv:      map[string]any{"report": 42},
			wantKinds:  []string{DeliverableFindingThinContent},
			wantAbsent: []string{DeliverableFindingEmptyContent},
		},
		{
			name:       "嵌套标量递归兑底",
			deliv:      map[string]any{"report": map[string]any{"score": 0.95, "ok": true}},
			wantKinds:  []string{DeliverableFindingThinContent},
			wantAbsent: []string{DeliverableFindingEmptyContent},
		},
		{
			// nil 不兑底为 "<nil>"，仍按 empty 判。
			name:      "nil 值仍判 empty",
			deliv:     map[string]any{"report": nil},
			wantKinds: []string{DeliverableFindingEmptyContent},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := InspectDeliverables(tc.deliv, tc.contract)
			kinds := map[string]bool{}
			for _, f := range got {
				kinds[f.Kind] = true
				if f.Topic == "" || f.Detail == "" {
					t.Errorf("finding 字段不完整: %+v", f)
				}
			}
			for _, k := range tc.wantKinds {
				if !kinds[k] {
					t.Errorf("期望 kind %q 出现，实际 findings=%+v", k, got)
				}
			}
			for _, k := range tc.wantAbsent {
				if kinds[k] {
					t.Errorf("期望 kind %q 不出现，实际 findings=%+v", k, got)
				}
			}
		})
	}
}
