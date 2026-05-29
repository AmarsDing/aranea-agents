package biz

import (
	"context"
	"encoding/hex"
	"os"
	"testing"
)

type channelRepoStub struct {
	channels    []Channel
	credentials map[string][]ChannelCredential
}

func (s *channelRepoStub) List(_ context.Context) ([]Channel, error) {
	return append([]Channel(nil), s.channels...), nil
}

func (s *channelRepoStub) Get(_ context.Context, id string) (Channel, error) {
	for _, c := range s.channels {
		if c.ID == id {
			return c, nil
		}
	}
	return Channel{}, channelValidationError("not found")
}

func (s *channelRepoStub) GetByKey(_ context.Context, channelKey string) (Channel, error) {
	for _, c := range s.channels {
		if c.Key == channelKey {
			return c, nil
		}
	}
	return Channel{}, channelValidationError("not found")
}

func (s *channelRepoStub) Create(_ context.Context, row Channel) (Channel, error) {
	s.channels = append(s.channels, row)
	return row, nil
}

func (s *channelRepoStub) Update(_ context.Context, row Channel) (Channel, error) {
	for i, c := range s.channels {
		if c.ID == row.ID {
			s.channels[i] = row
			return row, nil
		}
	}
	return row, nil
}

func (s *channelRepoStub) Delete(_ context.Context, id string) error { return nil }

func (s *channelRepoStub) ListCredentials(_ context.Context, channelID string) ([]ChannelCredential, error) {
	return append([]ChannelCredential(nil), s.credentials[channelID]...), nil
}

func (s *channelRepoStub) UpsertCredential(_ context.Context, cred ChannelCredential) (ChannelCredential, error) {
	if s.credentials == nil {
		s.credentials = map[string][]ChannelCredential{}
	}
	items := s.credentials[cred.ChannelID]
	for i, item := range items {
		if item.CredentialKey == cred.CredentialKey {
			items[i] = cred
			s.credentials[cred.ChannelID] = items
			return cred, nil
		}
	}
	s.credentials[cred.ChannelID] = append(items, cred)
	return cred, nil
}

func (s *channelRepoStub) DeleteCredential(_ context.Context, channelID, credentialKey string) error {
	return nil
}

func (s *channelRepoStub) ListDeliveries(_ context.Context, channelID string, limit int) ([]ChannelDelivery, error) {
	return nil, nil
}

func (s *channelRepoStub) AddDelivery(_ context.Context, d ChannelDelivery) (ChannelDelivery, error) {
	return d, nil
}

func (s *channelRepoStub) ListPendingDeliveries(_ context.Context, limit int) ([]ChannelDelivery, error) {
	return nil, nil
}

func (s *channelRepoStub) UpdateDelivery(_ context.Context, d ChannelDelivery) error {
	return nil
}

func TestChannelCredentialEncryptRoundTrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)
	c := NewCredentialCrypto(nil)

	ref, err := c.EncryptChannelSecretRef(context.Background(), "app-secret-value")
	if err != nil {
		t.Fatal(err)
	}
	if ref == "" || ref[:4] != "enc:" {
		t.Fatalf("unexpected ref %q", ref)
	}
	plain, err := c.DecryptChannelSecretRef(context.Background(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if plain != "app-secret-value" {
		t.Fatalf("got %q", plain)
	}
}

func TestChannelUpsertCredentialsStoresEncryptedRef(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	_ = os.Setenv(envCredentialKey, hex.EncodeToString(key))
	defer os.Unsetenv(envCredentialKey)

	repo := &channelRepoStub{
		channels: []Channel{{
			ID:         "ch-1",
			Key:        "feishu-demo",
			Name:       "Feishu",
			Status:     "active",
			Enabled:    true,
			ConfigJSON: `{"type":"feishu","receive_mode":"webhook","config":{"app_id":"cli_test"},"webhook":{"path":"/webhooks/feishu-demo"}}`,
		}},
	}
	uc := NewChannelUsecase(repo, NewCredentialCrypto(nil))
	items, err := uc.UpsertCredentials(context.Background(), "ch-1", []ChannelCredentialInput{{
		CredentialKey: "app_secret",
		Secret:        "super-secret",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || !items[0].Configured {
		t.Fatalf("unexpected items %#v", items)
	}
	raw, err := uc.ListCredentialsRaw(context.Background(), "ch-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 1 {
		t.Fatal("expected raw credential")
	}
	if raw[0].SecretRef[:4] != "enc:" {
		t.Fatalf("expected enc ref, got %q", raw[0].SecretRef)
	}
	plain, err := uc.crypto.DecryptChannelSecretRef(context.Background(), raw[0].SecretRef)
	if err != nil || plain != "super-secret" {
		t.Fatalf("decrypt: %v plain=%q", err, plain)
	}
}

func TestChannelEvaluateTestPendingAuth(t *testing.T) {
	row := Channel{
		Enabled:    true,
		ConfigJSON: `{"type":"feishu","receive_mode":"webhook","webhook":{"path":"/webhooks/x"}}`,
	}
	result, err := EvaluateChannelTest(row, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Status != "pending_auth" {
		t.Fatalf("got %#v", result)
	}
}

func TestChannelEvaluateTestFeishuMissingAppID(t *testing.T) {
	row := Channel{
		Enabled: true,
		ConfigJSON: `{"type":"feishu","receive_mode":"webhook","webhook":{"path":"/webhooks/x"},` +
			`"config":{"region":"feishu"}}`,
	}
	creds := []ChannelCredential{{
		CredentialKey: "app_secret",
		SecretRef:     "enc:deadbeef",
	}}
	result, err := EvaluateChannelTest(row, creds)
	if err != nil {
		t.Fatal(err)
	}
	if result.OK || result.Status != "pending_config" {
		t.Fatalf("got %#v", result)
	}
}

func TestChannelRunHealthChecksUpdatesStatus(t *testing.T) {
	repo := &channelRepoStub{
		channels: []Channel{{
			ID:           "ch-2",
			Key:          "ding",
			Name:         "DingTalk",
			Status:       "active",
			Enabled:      true,
			ConfigJSON:   `{"type":"dingtalk","receive_mode":"webhook","webhook":{"path":"/webhooks/ding"}}`,
			MetadataJSON: `{}`,
		}},
		credentials: map[string][]ChannelCredential{
			"ch-2": {{
				ChannelID:     "ch-2",
				CredentialKey: "app_secret",
				SecretRef:     "env:MISSING_VAR",
			}},
		},
	}
	uc := NewChannelUsecase(repo, NewCredentialCrypto(nil))
	if err := uc.RunHealthChecks(context.Background()); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.Get(context.Background(), "ch-2")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != "active" {
		t.Fatalf("status=%q", updated.Status)
	}
}

func TestChannelNormalizeRequiresType(t *testing.T) {
	row := Channel{Key: "k", Name: "n", ConfigJSON: `{"receive_mode":"webhook"}`}
	err := normalizeChannel(&row)
	if err == nil {
		t.Fatal("expected error")
	}
}
