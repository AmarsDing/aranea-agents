package biz

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// TestFilterMenuGroups 覆盖 P1-7 的可见性裁剪语义：
// 空 permission = 仅需登录（恒可见）；分组被裁空则整组省略；allow=nil 时不放行
// 任何带码条目（诚实失败，而非下发全量）。
func TestFilterMenuGroups(t *testing.T) {
	catalog := NavigationCatalog()
	all := FilterMenuGroups(catalog, func(string) bool { return true })
	if len(all) != len(catalog) {
		t.Fatalf("allow-all groups = %d, want %d", len(all), len(catalog))
	}
	if n, m := countItems(all), countItems(catalog); n != m {
		t.Fatalf("allow-all items = %d, want %d", n, m)
	}

	none := FilterMenuGroups(catalog, func(string) bool { return false })
	for _, g := range none {
		for _, it := range g.Items {
			if it.Permission != "" {
				t.Fatalf("ungranted item leaked into output: %s (%s)", it.Key, it.Permission)
			}
		}
	}
	if n, m := countItems(none), countItems(catalog); n >= m {
		t.Fatalf("deny-all items = %d, want fewer than catalog's %d (gated items must drop)", n, m)
	}

	// 无任何授权时：带码条目全部消失，仅剩「仅需登录」条目。
	// knowledge 组只剩 evaluation（无码），ops 组只剩 selfImprovement（无码）。
	if items := groupByKey(none, "knowledge"); len(items) != 1 || items[0].Key != "evaluation" {
		t.Fatalf("deny-all knowledge group = %+v, want only evaluation", items)
	}
	if items := groupByKey(none, "ops"); len(items) != 1 || items[0].Key != "selfImprovement" {
		t.Fatalf("deny-all ops group = %+v, want only selfImprovement", items)
	}

	// 只授予 knowledge.read：knowledge/artifacts 回归，其余仍被裁掉。
	granted := map[string]bool{"knowledge.read": true}
	partial := FilterMenuGroups(catalog, func(code string) bool { return granted[code] })
	if items := groupByKey(partial, "knowledge"); len(items) != 3 {
		t.Fatalf("knowledge group items = %d, want 3 (two gated + evaluation)", len(items))
	}
	if items := groupByKey(partial, "ops"); len(items) != 1 || items[0].Key != "selfImprovement" {
		t.Fatalf("ops group = %+v, want only selfImprovement", items)
	}
	if got := groupByKey(partial, "workspace"); len(got) != len(groupByKey(none, "workspace")) {
		t.Fatalf("workspace group changed under knowledge-only grant: %d", len(got))
	}

	// 整组裁空即省略：授予 knowledge.read 后 mediaCommerce 组无带码条目、
	// 故仍在（全为登录可见），但一个"只授予 knowledge.read、其余全带码"的
	// 场景下组应当消失——用构造目录直接验证省略语义。
	synthetic := []MenuGroup{{
		Key:   "g",
		Items: []MenuEntry{{Key: "a", Permission: "x.y"}, {Key: "b", Permission: "x.y"}},
	}}
	if out := FilterMenuGroups(synthetic, func(string) bool { return false }); len(out) != 0 {
		t.Fatalf("emptied group must be omitted, got %d groups", len(out))
	}

	// 值拷贝：改写返回值不得污染目录。
	first := FilterMenuGroups(catalog, func(string) bool { return true })
	first[0].Items[0].Path = "/mutated"
	if again := NavigationCatalog(); again[0].Items[0].Path == "/mutated" {
		t.Fatal("FilterMenuGroups leaked a mutable reference to the catalog")
	}
}

func countItems(groups []MenuGroup) int {
	n := 0
	for _, g := range groups {
		n += len(g.Items)
	}
	return n
}

func groupByKey(groups []MenuGroup, key string) []MenuEntry {
	for _, g := range groups {
		if g.Key == key {
			return g.Items
		}
	}
	return nil
}

// TestNavigationCatalogIntegrity 目录自身的完整性：Key/Path 唯一、i18n 键与图标
// 齐备、分组非空。
func TestNavigationCatalogIntegrity(t *testing.T) {
	seenKey := map[string]bool{}
	seenPath := map[string]bool{}
	for _, g := range NavigationCatalog() {
		if g.Key == "" || g.LabelKey == "" || len(g.Items) == 0 {
			t.Fatalf("malformed group %+v", g)
		}
		for _, it := range g.Items {
			if it.Key == "" || it.Path == "" || it.LabelKey == "" || it.Icon == "" {
				t.Fatalf("malformed item in group %s: %+v", g.Key, it)
			}
			if seenKey[it.Key] {
				t.Fatalf("duplicate item key %q", it.Key)
			}
			if seenPath[it.Path] {
				t.Fatalf("duplicate item path %q", it.Path)
			}
			seenKey[it.Key], seenPath[it.Path] = true, true
		}
	}
}

// sideNavItemRe 从 web/src/config/sideNav.ts 抽取菜单项的字面量。
var sideNavItemRe = regexp.MustCompile(`\{\s*to:\s*'([^']+)',\s*icon:\s*'([^']+)',\s*labelKey:\s*'([^']+)'([^}]*)\}`)

// TestNavigationCatalogMatchesWebSideNav 是 P1-7 的漂移守卫：Go 侧菜单目录与
// 前端 sideNav.ts 必须逐项一致（path/icon/labelKey/exact/permission）。任一侧
// 增删改菜单而另一侧未同步即失败——菜单是"下发内容"与"渲染内容"两份数据，
// 不锁死必然漂移。
func TestNavigationCatalogMatchesWebSideNav(t *testing.T) {
	path := filepath.Join("..", "..", "web", "src", "config", "sideNav.ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(raw)

	type webItem struct {
		path, icon, labelKey string
		exact                bool
		permission           string
	}
	var web []webItem
	for _, m := range sideNavItemRe.FindAllStringSubmatch(src, -1) {
		it := webItem{path: m[1], icon: m[2], labelKey: m[3]}
		rest := m[4]
		if strings.Contains(rest, "exact: false") {
			it.exact = false
		} else if strings.Contains(rest, "exact: true") {
			it.exact = true
		}
		if pm := regexp.MustCompile(`permission:\s*'([^']*)'`).FindStringSubmatch(rest); pm != nil {
			it.permission = pm[1]
		}
		web = append(web, it)
	}
	if len(web) == 0 {
		t.Fatalf("parsed 0 items from %s — parser or file shape changed", path)
	}

	var got []webItem
	for _, g := range NavigationCatalog() {
		for _, it := range g.Items {
			got = append(got, webItem{path: it.Path, icon: it.Icon, labelKey: it.LabelKey, exact: it.Exact, permission: it.Permission})
		}
	}

	key := func(it webItem) string {
		return strings.Join([]string{it.path, it.icon, it.labelKey, boolStr(it.exact), it.permission}, "|")
	}
	seen := map[string]bool{}
	for _, it := range web {
		seen[key(it)] = true
	}
	var missing []string // 目录有、web 无
	for _, it := range got {
		if !seen[key(it)] {
			missing = append(missing, key(it))
		}
	}
	// web 有、目录无：用 path 比对（其余字段的差异已在 missing 侧逐字段暴露）。
	gotPaths := map[string]bool{}
	for _, it := range got {
		gotPaths[it.path] = true
	}
	var extra []string
	for _, it := range web {
		if !gotPaths[it.path] {
			extra = append(extra, key(it))
		}
	}
	sort.Strings(missing)
	sort.Strings(extra)
	if len(missing) > 0 {
		t.Errorf("menu catalog entries absent from web/src/config/sideNav.ts (%d):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
	if len(extra) > 0 {
		t.Errorf("sideNav.ts entries absent from NavigationCatalog (%d):\n  %s", len(extra), strings.Join(extra, "\n  "))
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
