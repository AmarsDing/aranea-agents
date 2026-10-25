package toolspill

import (
	"strings"
	"testing"
)

// store_kind_test.go —— C-5 PG SpillStore env 档位解析矩阵
//（ResolveToolSpillStoreKind）：缺省/空=fs 字节级旧行为；大小写与空白
// 容忍；非法值回落 fs 且 malformed=true（fail-safe：不认得的档位=旧行为）。

func TestResolveToolSpillStoreKind_Matrix(t *testing.T) {
	cases := []struct {
		name      string
		value     string
		wantKind  string
		wantBogus bool
	}{
		{"unset-empty", "", StoreFS, false},
		{"fs-exact", "fs", StoreFS, false},
		{"fs-upper", "FS", StoreFS, false},
		{"fs-padded", "  fs  ", StoreFS, false},
		{"pg-exact", "pg", StorePG, false},
		{"pg-upper", "PG", StorePG, false},
		{"pg-padded", " Pg ", StorePG, false},
		{"bogus", "bogus", StoreFS, true},
		{"mem-not-a-kind", "mem", StoreFS, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			kind, malformed := ResolveToolSpillStoreKind(func(string) string { return c.value })
			if kind != c.wantKind || malformed != c.wantBogus {
				t.Fatalf("resolve %q = (%q,%v), want (%q,%v)", c.value, kind, malformed, c.wantKind, c.wantBogus)
			}
		})
	}
}

func TestResolveToolSpillStoreKind_NilGetenvFallsBackToOS(t *testing.T) {
	t.Setenv(EnvToolSpillStore, "pg")
	kind, malformed := ResolveToolSpillStoreKind(nil)
	if kind != StorePG || malformed {
		t.Fatalf("nil-getenv resolve = (%q,%v), want (pg,false)", kind, malformed)
	}
	t.Setenv(EnvToolSpillStore, "")
	kind, malformed = ResolveToolSpillStoreKind(nil)
	if kind != StoreFS || malformed {
		t.Fatalf("nil-getenv empty resolve = (%q,%v), want (fs,false)", kind, malformed)
	}
}

// TestIsSpillID C-f C5-PARITY-1：钉死两 store 共享的 id 语法（idLen
// 小写 hex），PG Get/Stat 经此判违例哨兵 ErrInvalidLocator（镜像 FS
// splitFSID 的 isHex/idLen 规则）。
func TestIsSpillID(t *testing.T) {
	valid := []string{
		strings.Repeat("a", 32),
		"0123456789abcdef0123456789abcdef",
	}
	invalid := []string{
		"",
		strings.Repeat("a", 31),
		strings.Repeat("a", 33),
		strings.Repeat("A", 32), // 大写 hex 不收
		strings.Repeat("z", 32),
		strings.Repeat("a", 16) + "/" + strings.Repeat("b", 16),
	}
	for _, id := range valid {
		if !IsSpillID(id) {
			t.Fatalf("IsSpillID(%q) = false, want true", id)
		}
	}
	for _, id := range invalid {
		if IsSpillID(id) {
			t.Fatalf("IsSpillID(%q) = true, want false", id)
		}
	}
}
