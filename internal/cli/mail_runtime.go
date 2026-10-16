package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/mail"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/secret"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

// newMailService wires SMTP delivery and turns on email codes for users. An
// empty master key still allows relays that need no password.
func newMailService(ctx context.Context, db *gorm.DB, cfg config.Config, users *service.UserService) (*service.MailService, error) {
	settings, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create mail settings repository: %w", err)
	}
	var codec service.SecretCodec
	if cfg.Security.MasterKey != "" {
		codec, err = secret.NewCodec(ctx, cfg.Security.MasterKey)
		if err != nil {
			return nil, fmt.Errorf("create mail encryption: %w", err)
		}
	}
	mailer, err := service.NewMailService(ctx, settings, codec, mail.NewSMTP(), settings)
	if err != nil {
		return nil, err
	}
	codes, err := repo.NewEmailCodeRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create email code repository: %w", err)
	}
	users.UseEmailCodes(codes, mailer, settings, time.Now)
	return mailer, nil
}
