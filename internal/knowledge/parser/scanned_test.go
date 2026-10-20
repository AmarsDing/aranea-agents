/*
U2-3 解析纵深：疑似扫描页检测回归——页锚切分 / 阈值判定 / 建议文案。
*/
package parser

import (
	"strings"
	"testing"
)

// 全部页正文充足：不误报，无建议。
func TestAnalyzeScannedPages_AllTextPages(t *testing.T) {
	md := `<!-- page:1 -->

这是一页正常的文本，包含足够多的正文字符，远超过疑似扫描的判定阈值。

<!-- page:2 -->

第二页同样是充分的文字内容，表格或段落都正常抽取出来了。`
	rep := AnalyzeScannedPages(md)
	if len(rep.Pages) != 0 {
		t.Fatalf("text pages falsely flagged: %+v", rep)
	}
	if rep.Suggestion != "" {
		t.Fatalf("suggestion should be empty, got %q", rep.Suggestion)
	}
}

// 中间一页正文碎片化：精确命中该页并给出视觉模型建议。
func TestAnalyzeScannedPages_SingleScannedPage(t *testing.T) {
	md := `<!-- page:1 -->

第一页正文充足，包含完整的段落与足够字符数，不属于疑似扫描页。

<!-- page:2 -->

扫

<!-- page:3 -->

第三页也是充足的正文内容，来自正常的文字层抽取结果输出。`
	rep := AnalyzeScannedPages(md)
	if len(rep.Pages) != 1 || rep.Pages[0] != 2 {
		t.Fatalf("Pages = %v, want [2]", rep.Pages)
	}
	if !strings.Contains(rep.Suggestion, "扫描图像") || !strings.Contains(rep.Suggestion, "视觉模型") {
		t.Fatalf("Suggestion = %q, want scan/vision guidance", rep.Suggestion)
	}
}

// 多页疑似 + 无锚输入不判定。
func TestAnalyzeScannedPages_MultipleScannedAndNoAnchor(t *testing.T) {
	md := `<!-- page:1 -->

水印

<!-- page:2 -->

第二页正文充足，包含完整的段落与足够字符数，不属于疑似扫描页范围。

<!-- page:3 -->

页脚碎片`
	rep := AnalyzeScannedPages(md)
	if len(rep.Pages) != 2 || rep.Pages[0] != 1 || rep.Pages[1] != 3 {
		t.Fatalf("Pages = %v, want [1,3]", rep.Pages)
	}
	if rep.Suggestion == "" {
		t.Fatal("Suggestion should be set when pages flagged")
	}
	// 无 <!-- page:N --> 锚的文本无权判定，原样返回空结果。
	if rep := AnalyzeScannedPages("plain text without any page anchor, long enough to matter"); len(rep.Pages) != 0 || rep.Suggestion != "" {
		t.Fatalf("no-anchor input should yield empty report, got %+v", rep)
	}
	// 空输入同样安全。
	if rep := AnalyzeScannedPages(""); len(rep.Pages) != 0 {
		t.Fatalf("empty input flagged: %+v", rep)
	}
}
