package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/captcha"
	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/secret"
	"github.com/biliblihuorong/imgnest/internal/service"
	"gorm.io/gorm"
)

// newCaptchaService always wires the native policy, even without a master key.
// An empty key permits the default disabled configuration, but cannot configure or
// enable CAPTCHA. Existing enabled state without a usable key remains fail-closed.
func newCaptchaService(ctx context.Context, db *gorm.DB, cfg config.Config) (*service.CaptchaService, error) {
	settings, err := repo.NewSettingsRepository(ctx, db)
	if err != nil {
		return nil, fmt.Errorf("create captcha repository: %w", err)
	}
	var codec service.SecretCodec
	if cfg.Security.MasterKey != "" {
		codec, err = secret.NewCodec(ctx, cfg.Security.MasterKey)
		if err != nil {
			return nil, fmt.Errorf("create captcha encryption: %w", service.ErrCaptchaUnavailable)
		}
	}
	return service.NewCaptchaService(ctx, service.CaptchaDependencies{
		Repository: settings, Secrets: codec, Verifier: captcha.NewTurnstile(), Now: time.Now,
	})
}
