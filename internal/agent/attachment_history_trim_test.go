package agent

// attachment_history_trim_test.go — T-1b 历史图片裁剪 + T-1c 图片计量钉板
// （2026-10-26 评审二轮）。

import (
	"context"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func imgMsg(data []byte) trpcmodel.Message {
	text := "见图"
	return trpcmodel.Message{
		Role: trpcmodel.RoleUser,
		ContentParts: []trpcmodel.ContentPart{
			{Type: trpcmodel.ContentTypeText, Text: &text},
			{Type: trpcmodel.ContentTypeImage, Image: &trpcmodel.Image{Data: data, Format: "png"}},
		},
	}
}

func textMsg(s string) trpcmodel.Message {
	return trpcmodel.Message{Role: trpcmodel.RoleUser, Content: s}
}

// T-1b①：最后一条带图消息保留原图，更早的图片 part 全部替换为文本占位。
func TestTrimHistoricalImageParts_KeepsLatestTrimsOlder(t *testing.T) {
	msgs := []trpcmodel.Message{
		imgMsg([]byte{1}),        // 历史带图 → 裁
		textMsg("中间轮"),          // 无图 → 不动
		imgMsg([]byte{2, 3}),     // 历史带图 → 裁
		imgMsg([]byte{4, 5, 6}),  // 最后一条带图 → 保留
	}
	trimmed := trimHistoricalImageParts(msgs)
	if trimmed != 2 {
		t.Fatalf("trimmed = %d, want 2", trimmed)
	}
	// 历史位：图片 part 变文本占位，文本 part 保留。
	for _, i := range []int{0, 2} {
		parts := msgs[i].ContentParts
		if len(parts) != 2 {
			t.Fatalf("msgs[%d] parts = %d, want 2", i, len(parts))
		}
		if parts[0].Type != trpcmodel.ContentTypeText || *parts[0].Text != "见图" {
			t.Fatalf("msgs[%d] text part clobbered: %+v", i, parts[0])
		}
		if parts[1].Type != trpcmodel.ContentTypeText || *parts[1].Text != imageHistoryPlaceholder {
			t.Fatalf("msgs[%d] image part not replaced by placeholder: %+v", i, parts[1])
		}
	}
	// 最后一条带图消息：原图原样保留。
	last := msgs[3].ContentParts[1]
	if last.Type != trpcmodel.ContentTypeImage || last.Image == nil || len(last.Image.Data) != 3 {
		t.Fatalf("latest image part must stay intact, got %+v", last)
	}
	// 无图消息不动。
	if msgs[1].Content != "中间轮" || len(msgs[1].ContentParts) != 0 {
		t.Fatalf("text-only message mutated: %+v", msgs[1])
	}
}

// T-1b②：唯一带图消息（首轮图片输入）不裁——它就是当前轮。
func TestTrimHistoricalImageParts_SingleImageUntouched(t *testing.T) {
	msgs := []trpcmodel.Message{textMsg("hi"), imgMsg([]byte{9})}
	if trimmed := trimHistoricalImageParts(msgs); trimmed != 0 {
		t.Fatalf("trimmed = %d, want 0", trimmed)
	}
	if msgs[1].ContentParts[1].Type != trpcmodel.ContentTypeImage {
		t.Fatal("sole image message must stay intact")
	}
}

// T-1b③：无图历史为纯 no-op。
func TestTrimHistoricalImageParts_NoImagesNoop(t *testing.T) {
	msgs := []trpcmodel.Message{textMsg("a"), textMsg("b")}
	if trimmed := trimHistoricalImageParts(msgs); trimmed != 0 {
		t.Fatalf("trimmed = %d, want 0", trimmed)
	}
}

// T-1b④：hook 接线——BeforeModel 回调改写 request 副本。
func TestImageHistoryTrimHook_RewritesRequestMessages(t *testing.T) {
	hook := newImageHistoryTrimBeforeHook(nil)
	if hook == nil {
		t.Fatal("trim hook must be unconditional")
	}
	msgs := []trpcmodel.Message{imgMsg([]byte{1}), imgMsg([]byte{2})}
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{Messages: msgs}}
	if _, err := hook.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if msgs[0].ContentParts[1].Type != trpcmodel.ContentTypeText {
		t.Fatal("historical image not trimmed by hook")
	}
	if msgs[1].ContentParts[1].Type != trpcmodel.ContentTypeImage {
		t.Fatal("latest image must survive the hook")
	}
}

// T-1c①：messageEstTokens 对图片 part 计入保守下限（此前计 0 → 压缩失明）。
func TestMessageEstTokens_ImageFloor(t *testing.T) {
	withImg := messageEstTokens(imgMsg([]byte{1}))
	textOnly := messageEstTokens(textMsg("见图"))
	if withImg-textOnly != imagePartEstTokensFloor {
		t.Fatalf("image floor delta = %d, want %d", withImg-textOnly, imagePartEstTokensFloor)
	}
}

// T-1c②：analyzePromptRequest 汇总含图片计量。
func TestAnalyzePromptRequest_CountsImages(t *testing.T) {
	report := analyzePromptRequest([]trpcmodel.Message{imgMsg([]byte{1}), textMsg("x")})
	if report.ImageParts != 1 {
		t.Fatalf("ImageParts = %d, want 1", report.ImageParts)
	}
	plain := analyzePromptRequest([]trpcmodel.Message{textMsg("见图"), textMsg("x")})
	if report.EstTokens-plain.EstTokens != imagePartEstTokensFloor {
		t.Fatalf("EstTokens image delta = %d, want %d",
			report.EstTokens-plain.EstTokens, imagePartEstTokensFloor)
	}
}

// T-1c③：分段估算（shadow 候选）同口径计入图片下限——防翻盘后重新失明。
func TestPromptEstTokensSegmented_ImageFloor(t *testing.T) {
	withImg := promptEstTokensSegmented([]trpcmodel.Message{imgMsg([]byte{1})}, SegmentedEstConfig{})
	plain := promptEstTokensSegmented([]trpcmodel.Message{textMsg("见图")}, SegmentedEstConfig{})
	if withImg-plain != imagePartEstTokensFloor {
		t.Fatalf("segmented image delta = %d, want %d", withImg-plain, imagePartEstTokensFloor)
	}
}
