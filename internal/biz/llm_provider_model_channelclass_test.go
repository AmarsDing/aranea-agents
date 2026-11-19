package biz

// D6 L-1 通道类别语义常量测试（NormalizeProviderChannelClass /
// ProviderChannelForcesLocal）。写入校验边界：空=cloud、大小写不敏感、
// 非法值 BadRequest（防拼写漂移静默吞掉密级语义）。

import (
	"testing"
)

func TestNormalizeProviderChannelClass(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"", "cloud", false},
		{"  ", "cloud", false},
		{"local", "local", false},
		{"LOCAL", "local", false},
		{" Local ", "local", false},
		{"cloud", "cloud", false},
		{"Cloud", "cloud", false},
		{"loca", "", true},
		{"onprem", "", true},
		{"local cloud", "", true},
	}
	for _, tc := range cases {
		got, err := NormalizeProviderChannelClass(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("NormalizeProviderChannelClass(%q) must error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("NormalizeProviderChannelClass(%q) unexpected err: %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("NormalizeProviderChannelClass(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestProviderChannelForcesLocal(t *testing.T) {
	if !ProviderChannelForcesLocal("local") || !ProviderChannelForcesLocal(" LOCAL ") {
		t.Fatal("local variants must force local")
	}
	for _, v := range []string{"", "cloud", "CLOUD", "loca", "onprem"} {
		if ProviderChannelForcesLocal(v) {
			t.Fatalf("ProviderChannelForcesLocal(%q) must be false", v)
		}
	}
}

func TestCeilingForcesLocal_Boundary(t *testing.T) {
	if !CeilingForcesLocal(3, 3) {
		t.Fatal("ceiling == threshold must force local (L3=秘密)")
	}
	if CeilingForcesLocal(2, 3) {
		t.Fatal("below threshold must not force local")
	}
	if CeilingForcesLocal(0, 3) {
		t.Fatal("unclassified must never force local")
	}
}
