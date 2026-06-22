package data_test

import (
	"context"
	"testing"

	bizavatar "aranea-agents/internal/biz/avatar"
	"aranea-agents/internal/data"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func TestRunAvatarImageRepairMigration_repairsCorruptedSystemAvatar(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	// Create a corrupted system avatar asset (not a valid JPEG).
	_, err := client.AvatarAsset.Create().
		SetID("avatar_career_01").
		SetAssetKey("avatar_career_01").
		SetName("职场 1").
		SetMimeType("image/jpeg").
		SetIsSystem(true).
		SetEnabled(true).
		SetStatus("active").
		SetSource("system").
		SetCategory("agent").
		SetWidthPx(512).
		SetHeightPx(512).
		SetFileSizeBytes(42).
		SetImageData([]byte{0xEF, 0xBF, 0xBD, 0x00, 0x01, 0x02}).
		SetThumbnailData([]byte{0xEF, 0xBF, 0xBD}).
		SetCreatedAt("2026-01-01T00:00:00Z").
		SetUpdatedAt("2026-01-01T00:00:00Z").
		Save(ctx)
	if err != nil {
		t.Fatalf("create corrupted avatar: %v", err)
	}

	if err := data.RunAvatarImageRepairMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop()); err != nil {
		t.Fatalf("RunAvatarImageRepairMigration: %v", err)
	}

	po, err := client.AvatarAsset.Get(ctx, "avatar_career_01")
	if err != nil {
		t.Fatalf("get repaired avatar: %v", err)
	}
	if len(po.ImageData) < 3 || po.ImageData[0] != 0xFF || po.ImageData[1] != 0xD8 || po.ImageData[2] != 0xFF {
		t.Fatalf("image_data was not repaired to a valid JPEG, got first bytes=%02x %02x %02x", po.ImageData[0], po.ImageData[1], po.ImageData[2])
	}
	if po.MimeType != "image/jpeg" {
		t.Fatalf("mime_type=%q want image/jpeg", po.MimeType)
	}
	if po.WidthPx != bizavatar.AvatarMainMaxPx {
		t.Fatalf("width_px=%d want %d", po.WidthPx, bizavatar.AvatarMainMaxPx)
	}
	if len(po.ThumbnailData) < 3 || po.ThumbnailData[0] != 0xFF || po.ThumbnailData[1] != 0xD8 {
		t.Fatalf("thumbnail_data was not repaired to a valid JPEG")
	}
}

func TestRunAvatarImageRepairMigration_skipsValidJPEG(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	validJPEG := []byte{0xFF, 0xD8, 0xFF, 0xDB, 0x00, 0x01}
	_, err := client.AvatarAsset.Create().
		SetID("avatar_valid_test").
		SetAssetKey("avatar_valid_test").
		SetName("Valid").
		SetMimeType("image/jpeg").
		SetIsSystem(true).
		SetEnabled(true).
		SetStatus("active").
		SetSource("system").
		SetCategory("agent").
		SetWidthPx(100).
		SetHeightPx(100).
		SetFileSizeBytes(len(validJPEG)).
		SetImageData(validJPEG).
		SetCreatedAt("2026-01-01T00:00:00Z").
		SetUpdatedAt("2026-01-01T00:00:00Z").
		Save(ctx)
	if err != nil {
		t.Fatalf("create valid avatar: %v", err)
	}

	if err := data.RunAvatarImageRepairMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop()); err != nil {
		t.Fatalf("RunAvatarImageRepairMigration: %v", err)
	}

	po, err := client.AvatarAsset.Get(ctx, "avatar_valid_test")
	if err != nil {
		t.Fatalf("get avatar: %v", err)
	}
	if po.WidthPx != 100 || po.FileSizeBytes != len(validJPEG) {
		t.Fatalf("valid avatar was incorrectly modified: width=%d size=%d", po.WidthPx, po.FileSizeBytes)
	}
}

func TestRunAvatarImageRepairMigration_idempotent(t *testing.T) {
	client, _ := testhelper.SetupTestDB(t)
	ctx := context.Background()

	_, err := client.AvatarAsset.Create().
		SetID("avatar_career_02").
		SetAssetKey("avatar_career_02").
		SetName("职场 2").
		SetMimeType("image/jpeg").
		SetIsSystem(true).
		SetEnabled(true).
		SetStatus("active").
		SetSource("system").
		SetCategory("agent").
		SetWidthPx(512).
		SetHeightPx(512).
		SetFileSizeBytes(42).
		SetImageData([]byte{0xEF, 0xBF, 0xBD}).
		SetCreatedAt("2026-01-01T00:00:00Z").
		SetUpdatedAt("2026-01-01T00:00:00Z").
		Save(ctx)
	if err != nil {
		t.Fatalf("create corrupted avatar: %v", err)
	}

	if err := data.RunAvatarImageRepairMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop()); err != nil {
		t.Fatalf("first RunAvatarImageRepairMigration: %v", err)
	}
	first, err := client.AvatarAsset.Get(ctx, "avatar_career_02")
	if err != nil {
		t.Fatalf("get first: %v", err)
	}

	if err := data.RunAvatarImageRepairMigration(ctx, client, data.DialectSQLite, loggateway.NewNoop()); err != nil {
		t.Fatalf("second RunAvatarImageRepairMigration: %v", err)
	}
	second, err := client.AvatarAsset.Get(ctx, "avatar_career_02")
	if err != nil {
		t.Fatalf("get second: %v", err)
	}

	if string(first.ImageData) != string(second.ImageData) {
		t.Fatalf("migration is not idempotent")
	}
}
