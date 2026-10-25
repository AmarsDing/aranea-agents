package agent

import (
	"testing"
)

// ---------------------------------------------------------------------------
// D7-F3：parseStreamSubTask 持久化处归一 DomainPath——泛词折叠「其他」、
// 空白裁剪、词表子域折叠主域，读侧（L0/L1/花名册）不再逐点防御。
// ---------------------------------------------------------------------------

func TestParseStreamSubTask_NormalizesDomainPath(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"空白裁剪", "创作 / 文学", "创作/文学"},
		{"词表子域折叠主域", "创作/诗歌", "创作"},
		{"泛词折入其他", "随便", "其他"},
		{"未知域折入其他", "量子烹饪学", "其他"},
		{"空路径保持空", "", ""},
		{"已归一路径幂等", "创作/文学", "创作/文学"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			remap := map[string]string{}
			st, err := parseStreamSubTask(
				`{"id":"s1","name":"n1","domain_path":"`+tc.raw+`"}`, remap, "tp_test")
			if err != nil {
				t.Fatalf("parseStreamSubTask: %v", err)
			}
			if st.DomainPath != tc.want {
				t.Fatalf("DomainPath = %q, want %q（归一后在持久化处收敛）", st.DomainPath, tc.want)
			}
		})
	}
}
