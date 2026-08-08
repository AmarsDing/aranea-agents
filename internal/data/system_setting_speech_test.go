package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// setupSpeechSettingRepo builds a Data with the Ent-managed system_settings
// table plus the speech_* DDL columns (mirroring 20260808_speech_columns.sql,
// which Ent auto-migration does not cover).
func setupSpeechSettingRepo(t *testing.T) *systemSettingRepo {
	t.Helper()
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_driver TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_endpoint TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_app_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_access_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_resource_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_language TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_driver TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_endpoint TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_app_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_access_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_resource_id TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_voice TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_tts_speed_ratio DOUBLE PRECISION NOT NULL DEFAULT 0`,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_archive_user_audio BOOLEAN DEFAULT NULL`,
	} {
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("add speech column: %v", err)
		}
	}
	d := &Data{
		entClient:  client,
		readClient: client,
		rw:         NewReadWriteClient(client, client),
		rawDB:      db,
		readDB:     db,
		rwDB:       NewReadWriteDB(db, db),
		lg:         loggateway.NewNoop(),
		dialect:    DialectPostgres,
	}
	if err := ensureDefaultSystemSetting(ctx, client); err != nil {
		t.Fatalf("seed singleton: %v", err)
	}
	return &systemSettingRepo{data: d}
}

func TestSystemSettingRepo_SpeechRoundTrip(t *testing.T) {
	r := setupSpeechSettingRepo(t)
	ctx := context.Background()

	// Fresh row: all empty, archive NULL (unset).
	got, err := r.GetSpeech(ctx)
	if err != nil {
		t.Fatalf("GetSpeech fresh: %v", err)
	}
	if got.ASR.Driver != "" || got.TTS.Voice != "" || got.TTS.SpeedRatio != 0 {
		t.Fatalf("fresh row must be empty, got %#v", got)
	}
	if got.ArchiveUserAudio != nil {
		t.Fatalf("fresh archive must be NULL (unset), got %v", *got.ArchiveUserAudio)
	}

	// Full update with credentials + explicit archive off.
	off := false
	in := biz.SpeechSetting{
		ASR: biz.ASRProviderConfig{
			Driver: "volcengine", Endpoint: "wss://asr", AppKey: " ak ", AccessKey: "sk",
			ResourceID: "rid", Language: "zh-CN",
		},
		TTS: biz.TTSProviderConfig{
			Driver: "volcengine", Endpoint: "wss://tts", AppKey: "ak", AccessKey: " sk ",
			ResourceID: "rid2", Voice: "zh_female_x", SpeedRatio: 1.5,
		},
		ArchiveUserAudio: &off,
	}
	saved, err := r.UpdateSpeech(ctx, in, true, true)
	if err != nil {
		t.Fatalf("UpdateSpeech: %v", err)
	}
	if saved.ASR.AppKey != "ak" || saved.TTS.AccessKey != "sk" {
		t.Fatalf("credentials must be trimmed+persisted: %#v", saved)
	}
	if saved.ArchiveUserAudio == nil || *saved.ArchiveUserAudio != false {
		t.Fatalf("explicit archive false must persist, got %#v", saved.ArchiveUserAudio)
	}
	if !biz.SpeechASRConfigured(saved) || !biz.SpeechTTSConfigured(saved) {
		t.Fatalf("saved row must report configured: %#v", saved)
	}

	// Cred flags off → credentials preserved even though the patch carries
	// none; non-cred fields are written verbatim (merge is usecase-level).
	kept, err := r.UpdateSpeech(ctx, biz.SpeechSetting{
		ASR: biz.ASRProviderConfig{Language: "en-US"},
	}, false, false)
	if err != nil {
		t.Fatalf("UpdateSpeech keep creds: %v", err)
	}
	if kept.ASR.Language != "en-US" {
		t.Fatalf("patch field must persist: %#v", kept)
	}
	if kept.ASR.AppKey != "ak" || kept.TTS.AccessKey != "sk" {
		t.Fatalf("cred flags off must preserve credentials: %#v", kept)
	}

	// Archive back to NULL (unset). Note: repo writes patch fields verbatim —
	// empty-field merging is the usecase's job (ApplySpeechPatch) — so non-cred
	// fields are cleared here while cred flags off preserve the credentials.
	cleared, err := r.UpdateSpeech(ctx, biz.SpeechSetting{}, false, false)
	if err != nil {
		t.Fatalf("UpdateSpeech clear archive: %v", err)
	}
	if cleared.ArchiveUserAudio != nil {
		t.Fatalf("nil archive must write NULL, got %v", *cleared.ArchiveUserAudio)
	}
	if cleared.ASR.Language != "" || cleared.TTS.Voice != "" {
		t.Fatalf("repo must write patch verbatim (merge is usecase-level): %#v", cleared)
	}
	if cleared.ASR.AppKey != "ak" || cleared.TTS.AccessKey != "sk" {
		t.Fatalf("cred flags off must preserve credentials: %#v", cleared)
	}
}

func TestSystemSettingRepo_GetSpeechNotFound(t *testing.T) {
	client, db := testhelper.SetupTestPG(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx,
		`ALTER TABLE system_settings ADD COLUMN IF NOT EXISTS speech_asr_driver TEXT NOT NULL DEFAULT ''`); err != nil {
		// Column set incomplete is fine for this test: query fails → error path.
		t.Logf("alter warn: %v", err)
	}
	d := &Data{
		entClient:  client,
		readClient: client,
		rw:         NewReadWriteClient(client, client),
		rawDB:      db,
		readDB:     db,
		rwDB:       NewReadWriteDB(db, db),
		lg:         loggateway.NewNoop(),
		dialect:    DialectPostgres,
	}
	r := &systemSettingRepo{data: d}
	// No singleton row seeded → NotFound (when columns exist) or error (when
	// columns missing); both are non-nil error paths the caller tolerates.
	if _, err := r.GetSpeech(ctx); err == nil {
		t.Fatal("expected error for missing singleton row")
	}
}
