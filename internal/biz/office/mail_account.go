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
	if err := normalizeMailAccountSMTP(a); err != nil {
		return nil, err
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
	if v := strings.TrimSpace(patch.Email); v != "" {
		cur.Email = v
	}
	if v := strings.TrimSpace(patch.Provider); v != "" {
		cur.Provider = v
	}
	if v := strings.TrimSpace(patch.SMTPHost); v != "" {
		cur.SMTPHost = v
	}
	if patch.SMTPPort > 0 {
		cur.SMTPPort = patch.SMTPPort
	}
	if v := strings.TrimSpace(patch.SMTPTLS); v != "" {
		cur.SMTPTLS = v
	}
	if err := normalizeMailAccountSMTP(cur); err != nil {
		return nil, err
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

// normalizeMailAccountSMTP 发件地址缺省取用户名（是邮箱时）；SMTP 没填时按服务商预设补；
// SMTP 端口按加密方式补默认（tls 465 / starttls 587 / none 25）。
func normalizeMailAccountSMTP(a *MailAccount) error {
	a.Email = strings.TrimSpace(a.Email)
	if a.Email == "" && strings.Contains(a.Username, "@") {
		a.Email = a.Username
	}
	a.SMTPHost = strings.TrimSpace(a.SMTPHost)
	if a.SMTPHost == "" {
		if p, ok := MailPresetByKey(a.Provider); ok {
			a.SMTPHost, a.SMTPPort, a.SMTPTLS = p.SMTPHost, p.SMTPPort, p.SMTPTLS
		}
	}
	if a.SMTPHost == "" {
		a.SMTPTLS, a.SMTPPort = "", 0
		return nil
	}
	mode, err := normalizeMailTLSMode(a.SMTPTLS)
	if err != nil {
		return err
	}
	a.SMTPTLS = mode
	if a.SMTPPort <= 0 {
		switch mode {
		case MailTLSTLS:
			a.SMTPPort = 465
		case MailTLSStartTLS:
			a.SMTPPort = 587
		default:
			a.SMTPPort = 25
		}
	}
	return nil
}

func defaultMailPort(mode string) int {
	if mode == MailTLSTLS {
		return 993
	}
	return 143
}
