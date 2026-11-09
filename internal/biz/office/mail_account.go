package office

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/pkg/loggateway"
)

// MailAccountUsecase 邮箱账号 CRUD（E-1：密码写后即弃——入参 password 加密为
// password_enc 落库，任何出参永不含明文/密文；本服务无 Reveal RPC，验收 #6）。
type MailAccountUsecase struct {
	accounts MailAccountRepo
	crypto   MailSecretCrypto
	lg       loggateway.Logger
	now      func() time.Time
}

func NewMailAccountUsecase(accounts MailAccountRepo, crypto MailSecretCrypto, lg loggateway.Logger) *MailAccountUsecase {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &MailAccountUsecase{accounts: accounts, crypto: crypto, lg: lg, now: time.Now}
}

// Create 建账号：host/username/password 必填；port=0 按 tlsMode 补默认
// （tls→993，starttls/none→143）；tlsMode 缺省 tls。
func (u *MailAccountUsecase) Create(ctx context.Context, a *MailAccount, plainPassword string) (*MailAccount, error) {
	a.Host = strings.TrimSpace(a.Host)
	a.Username = strings.TrimSpace(a.Username)
	if a.Host == "" || a.Username == "" || strings.TrimSpace(plainPassword) == "" {
		return nil, ErrMailHostRequired
	}
	mode, err := normalizeMailTLSMode(a.TLSMode)
	if err != nil {
		return nil, err
	}
	a.TLSMode = mode
	if a.Port <= 0 {
		a.Port = defaultMailPort(mode)
	}
	enc, err := u.crypto.EncryptSecret(ctx, plainPassword)
	if err != nil {
		return nil, err
	}
	a.PasswordEnc = enc
	if a.ID == "" {
		a.ID = "omac_" + uuid.NewString()
	}
	now := u.now()
	a.CreatedAt = now
	a.UpdatedAt = now
	if err := u.accounts.CreateMailAccount(ctx, a); err != nil {
		return nil, err
	}
	return a, nil
}

// Update 改账号（部分字段；password 非空=轮换，重新加密落库）。
func (u *MailAccountUsecase) Update(ctx context.Context, id, userID string, patch *MailAccount, plainPassword string) (*MailAccount, error) {
	cur, err := u.accounts.GetMailAccount(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if v := strings.TrimSpace(patch.Host); v != "" {
		cur.Host = v
	}
	if v := strings.TrimSpace(patch.Username); v != "" {
		cur.Username = v
	}
	if v := strings.TrimSpace(patch.Name); v != "" {
		cur.Name = v
	}
	if patch.Port > 0 {
		cur.Port = patch.Port
	}
	if v := strings.TrimSpace(patch.TLSMode); v != "" {
		mode, err := normalizeMailTLSMode(v)
		if err != nil {
			return nil, err
		}
		cur.TLSMode = mode
	}
	if strings.TrimSpace(plainPassword) != "" {
		enc, err := u.crypto.EncryptSecret(ctx, plainPassword)
		if err != nil {
			return nil, err
		}
		cur.PasswordEnc = enc
	}
	cur.UpdatedAt = u.now()
	if err := u.accounts.UpdateMailAccount(ctx, cur); err != nil {
		return nil, err
	}
	return cur, nil
}

func (u *MailAccountUsecase) Delete(ctx context.Context, id, userID string) error {
	return u.accounts.DeleteMailAccount(ctx, id, userID)
}

func (u *MailAccountUsecase) List(ctx context.Context, userID string) ([]*MailAccount, error) {
	return u.accounts.ListMailAccounts(ctx, userID)
}

func (u *MailAccountUsecase) Get(ctx context.Context, id, userID string) (*MailAccount, error) {
	return u.accounts.GetMailAccount(ctx, id, userID)
}

func normalizeMailTLSMode(mode string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "", MailTLSTLS:
		return MailTLSTLS, nil
	case MailTLSStartTLS:
		return MailTLSStartTLS, nil
	case MailTLSNone:
		return MailTLSNone, nil
	}
	return "", ErrMailTLSModeInvalid
}

func defaultMailPort(mode string) int {
	if mode == MailTLSTLS {
		return 993
	}
	return 143
}
