package biz

// P0-7 批 D1 钉板：ResolveSkillTrustForPersist（三处写入路径共用）与
// SkillSourceForSyncOrigin（落列口径=回填 SQL 同源）矩阵。钉死三条铁律：
//  1. provenance 是 source 列的唯一真源——manifest 声明的 source 绝不覆盖
//     落列值（供应链提权护栏：声明 builtin 不能让 zip 包变成内置）；
//  2. 声明只能更严、永不更宽——来源默认档与声明档恒取 StricterTrust；
//  3. 未知/空声明静默忽略（不视为声明），未知 syncOrigin 归 user_created。

import "testing"

func TestSkillSourceForSyncOrigin(t *testing.T) {
	cases := []struct {
		name       string
		syncOrigin string
		want       SkillSource
	}{
		{"import 归 zip_import", "import", SkillSourceZipImport},
		{"import 大小写免疫", " IMPORT ", SkillSourceZipImport},
		{"manual 归 user_created", "manual", SkillSourceUserCreated},
		{"filesystem 归 user_created", "filesystem", SkillSourceUserCreated},
		{"未知归 user_created", "something-else", SkillSourceUserCreated},
		{"空归 user_created", "", SkillSourceUserCreated},
		{"空白归 user_created", "   ", SkillSourceUserCreated},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SkillSourceForSyncOrigin(tc.syncOrigin); got != tc.want {
				t.Fatalf("SkillSourceForSyncOrigin(%q) = %q, want %q", tc.syncOrigin, got, tc.want)
			}
		})
	}
}

func TestResolveSkillTrustForPersist(t *testing.T) {
	cases := []struct {
		name           string
		syncOrigin     string
		declaredSource string
		declaredTrust  string
		wantSource     SkillSource
		wantTrust      SkillTrustLevel
	}{
		{"import 裸 → zip_import/T4", "import", "", "", SkillSourceZipImport, SkillTrustT4Untrusted},
		{"manual 裸 → user_created/T3", "manual", "", "", SkillSourceUserCreated, SkillTrustT3Standard},
		{"filesystem 裸 → user_created/T3", "filesystem", "", "", SkillSourceUserCreated, SkillTrustT3Standard},
		{"未知 origin 裸 → user_created/T3", "anything", "", "", SkillSourceUserCreated, SkillTrustT3Standard},
		{"声明 builtin 不提权（import 保持 T4）", "import", "builtin", "", SkillSourceZipImport, SkillTrustT4Untrusted},
		{"声明 zip_import 使 manual 更严 T4", "manual", "zip_import", "", SkillSourceUserCreated, SkillTrustT4Untrusted},
		{"声明 source 绝不覆盖 provenance", "manual", "zip_import", "", SkillSourceUserCreated, SkillTrustT4Untrusted},
		{"声明 trust T4 更严生效", "manual", "", "t4", SkillSourceUserCreated, SkillTrustT4Untrusted},
		{"声明 trust T1 不放宽来源默认", "manual", "", "T1", SkillSourceUserCreated, SkillTrustT3Standard},
		{"声明 trust T2 不放宽来源默认", "manual", "", "t2", SkillSourceUserCreated, SkillTrustT3Standard},
		{"声明 trust 未知静默忽略", "manual", "", "bogus", SkillSourceUserCreated, SkillTrustT3Standard},
		{"声明 source 未知静默忽略", "manual", "north-star", "", SkillSourceUserCreated, SkillTrustT3Standard},
		{"双声明组合取最严", "import", "user_created", "T2", SkillSourceZipImport, SkillTrustT4Untrusted},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotSource, gotTrust := ResolveSkillTrustForPersist(tc.syncOrigin, tc.declaredSource, tc.declaredTrust)
			if gotSource != tc.wantSource || gotTrust != tc.wantTrust {
				t.Fatalf("ResolveSkillTrustForPersist(%q, %q, %q) = (%q, %q), want (%q, %q)",
					tc.syncOrigin, tc.declaredSource, tc.declaredTrust,
					gotSource, gotTrust, tc.wantSource, tc.wantTrust)
			}
		})
	}
}
