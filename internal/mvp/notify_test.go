package mvp_test

import (
	"context"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz/review"
	"aranea-agents/internal/mvp"
)

func TestDefaultReviewNotify_PrivateURLNoop(t *testing.T) {
	mvp.ResetForTest()
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_URL", "http://127.0.0.1/hook")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_CHANNEL_ID", "")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_RECIPIENT", "")
	fn := mvp.DefaultReviewNotifyFromEnv()
	if fn != nil {
		t.Fatal("private/loopback notify URL must not install HTTP notify")
	}
}

func TestDefaultReviewNotify_PublicURLInstalls(t *testing.T) {
	mvp.ResetForTest()
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_URL", "https://example.com/review-pending")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_CHANNEL_ID", "")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_RECIPIENT", "")
	fn := mvp.DefaultReviewNotifyFromEnv()
	if fn == nil {
		t.Fatal("expected HTTP notify installed for public HTTPS URL")
	}
}

func TestDefaultReviewNotify_NativeChannel(t *testing.T) {
	mvp.ResetForTest()
	var called atomic.Int32
	var gotChannel, gotRecipient, gotText string
	mvp.ConfigureChannelEnqueue(func(ctx context.Context, channelID, platform, recipient, text, idempotencyKey string) error {
		called.Add(1)
		gotChannel, gotRecipient, gotText = channelID, recipient, text
		if platform != "feishu" {
			t.Errorf("platform=%q", platform)
		}
		if idempotencyKey == "" {
			t.Error("idempotency empty")
		}
		return nil
	})
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_URL", "")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_CHANNEL_ID", "ch-1")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_RECIPIENT", "ou_user")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_PLATFORM", "feishu")
	mvp.ConfigureReviewNotify(mvp.DefaultReviewNotifyFromEnv())
	task, err := mvp.Ensure().Review.Enqueue(review.Task{
		ContentItemID: "c1",
		DraftTitle:    "待审稿",
		DraftBody:     "body",
	})
	if err != nil {
		t.Fatal(err)
	}
	if called.Load() != 1 {
		t.Fatalf("channel notify calls=%d", called.Load())
	}
	if gotChannel != "ch-1" || gotRecipient != "ou_user" {
		t.Fatalf("channel=%q recipient=%q", gotChannel, gotRecipient)
	}
	want := mvp.NotifyPendingReviewMessage(task)
	if gotText != want {
		t.Fatalf("text=%q want=%q", gotText, want)
	}
}

func TestDefaultReviewNotify_ChannelEnvNoopWithoutEnqueue(t *testing.T) {
	mvp.ResetForTest()
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_URL", "")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_CHANNEL_ID", "ch-1")
	t.Setenv("ARANEA_MVP_REVIEW_NOTIFY_RECIPIENT", "ou_user")
	mvp.ConfigureReviewNotify(mvp.DefaultReviewNotifyFromEnv())
	if _, err := mvp.Ensure().Review.Enqueue(review.Task{
		ContentItemID: "c1",
		DraftTitle:    "t",
		DraftBody:     "b",
	}); err != nil {
		t.Fatal(err)
	}
	// No panic / no enqueue wired → noop-safe.
}

func TestComposeReviewNotify(t *testing.T) {
	var a, b int
	fn := mvp.ComposeReviewNotify(
		func(review.Task) { a++ },
		nil,
		func(review.Task) { b++ },
	)
	fn(review.Task{ID: "x"})
	if a != 1 || b != 1 {
		t.Fatalf("a=%d b=%d", a, b)
	}
	if mvp.ComposeReviewNotify(nil, nil) != nil {
		t.Fatal("expected nil compose")
	}
}
