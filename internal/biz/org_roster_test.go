package biz

import "testing"

func TestInferDomainPath_PackPositions(t *testing.T) {
	cases := []struct {
		pos, dept, name, want string
	}{
		{"xiaohongshu_specialist", "media_operations", "小红书运营专家", "创作/文案"},
		{"backend_architect", "backend_dev", "后端架构师", "软件/后端"},
		{"fault_diagnostician", "diagnostics", "故障诊断专家", "运维/诊断"},
		{"alert_handler", "alert_response", "告警处理专家", "运维/告警"},
		{"frontend_developer", "frontend_dev", "前端开发工程师", "软件/前端"},
		{"meeting_notes_specialist", "project_management", "会议纪要专家", "办公/文档"},
		{"qa_engineer", "gis_solutions", "GIS QA 工程师", "数据/空间"},
		{"data_engineer", "backend_dev", "数据工程师", "数据/分析"},
	}
	for _, tc := range cases {
		if got := InferDomainPath(tc.pos, tc.dept, tc.name); got != tc.want {
			t.Errorf("InferDomainPath(%q,%q) = %q, want %q", tc.pos, tc.dept, got, tc.want)
		}
	}
}

func TestDepartmentDomainPaths_KnownKeys(t *testing.T) {
	if got := DepartmentDomainPaths("media_operations", "媒体运营部"); got[0] != "创作/文案" {
		t.Fatalf("media_operations = %v", got)
	}
	if got := DepartmentDomainPaths("alert_response", "告警响应部"); got[0] != "运维/告警" {
		t.Fatalf("alert_response = %v", got)
	}
	if got := DepartmentDomainPaths("unknown_dept", "未知部"); len(got) != 0 {
		t.Fatalf("unknown should be empty, got %v", got)
	}
}

func TestInferMissionStatement_EmptyDescriptionStaysEmpty(t *testing.T) {
	if got := InferMissionStatement("文案", ""); got != "" {
		t.Fatalf("empty description must not invent mission, got %q", got)
	}
	if got := InferMissionStatement("文案", "小红书种草与生活方式内容"); !containsAny(got, "小红书") {
		t.Fatalf("got %q", got)
	}
}

func TestInferToolsProfile(t *testing.T) {
	if InferToolsProfile("创作/文案") != "research" {
		t.Fatal("content should be research")
	}
	if InferToolsProfile("软件/后端") != "coding" {
		t.Fatal("backend should be coding")
	}
	if InferToolsProfile("运维/诊断") != "coding" {
		t.Fatal("ops should be coding")
	}
}
