package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestServiceConstructorsValidateDependencies(t *testing.T) {
	if _, err := service.NewUserService(t.Context(), nil, nil); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("nil user-service dependencies error=%v", err)
	}
	if _, err := service.NewTokenService(
		t.Context(),
		nil,
		nil,
		nil,
	); !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("nil token-service dependencies error=%v", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := service.NewUserService(ctx, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled user-service construction error=%v", err)
	}
	if _, err := service.NewTokenService(
		ctx,
		nil,
		nil,
		nil,
	); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled token-service construction error=%v", err)
	}
}

type failingSettings struct{ err error }

func (settings failingSettings) RegistrationEnabled(context.Context) (bool, error) {
	return false, settings.err
}

func (settings failingSettings) DefaultGroupID(context.Context) (uint64, error) {
	return 0, settings.err
}

type unavailableUsers struct{ service.UserRepository }

type groupFailure struct{ err error }

func (settings groupFailure) RegistrationEnabled(context.Context) (bool, error) {
	return true, nil
}

func (settings groupFailure) DefaultGroupID(context.Context) (uint64, error) {
	return 0, settings.err
}

func TestInvalidRegistrationStopsBeforeGroupLookup(t *testing.T) {
	svc, err := service.NewUserService(t.Context(), unavailableUsers{}, groupFailure{err: errors.New("group unavailable")})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		input service.RegisterInput
	}{
		{
			name:  "short username",
			input: service.RegisterInput{Username: "ab", Email: "alice@example.com", Password: testPassword},
		},
		{
			name:  "display email",
			input: service.RegisterInput{Username: "alice", Email: "Alice <alice@example.com>", Password: testPassword},
		},
		{
			name:  "short password",
			input: service.RegisterInput{Username: "alice", Email: "alice@example.com", Password: "short"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.Register(t.Context(), tc.input); !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("invalid registration reached group lookup: %v", err)
			}
		})
	}
}

func TestRegisterGroupFailureIsPreserved(t *testing.T) {
	want := errors.New("group unavailable")
	svc, err := service.NewUserService(t.Context(), unavailableUsers{}, groupFailure{err: want})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Register(t.Context(), service.RegisterInput{
		Username: "alice", Email: "alice@example.com", Password: testPassword,
	})
	if !errors.Is(err, want) {
		t.Fatalf("registration lost group lookup failure: %v", err)
	}
}

type userLookupFailure struct {
	service.UserRepository
	err error
}

func (failure userLookupFailure) FindUserByEmail(context.Context, string) (model.User, error) {
	return model.User{}, failure.err
}

func (failure userLookupFailure) FindUserByID(context.Context, uint64) (model.User, error) {
	return model.User{}, failure.err
}

func TestCredentialLookupFailureIsPreserved(t *testing.T) {
	want := errors.New("user lookup unavailable")
	svc, err := service.NewUserService(t.Context(), userLookupFailure{err: want}, failingSettings{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.VerifyCredentials(t.Context(), "alice@example.com", testPassword); !errors.Is(err, want) {
		t.Fatalf("verification lost lookup failure: %v", err)
	}
	missing, err := service.NewUserService(t.Context(), userLookupFailure{err: service.ErrNotFound}, failingSettings{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = missing.VerifyCredentials(t.Context(), "alice@example.com", testPassword)
	if !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("missing account leaked lookup result: %v", err)
	}
}

func TestTokenListUserLookupFailureIsPreserved(t *testing.T) {
	want := errors.New("user lookup unavailable")
	svc, err := service.NewTokenService(
		t.Context(),
		touchFailure{},
		userLookupFailure{err: want},
		func() time.Time { return time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.List(t.Context(), 1); !errors.Is(err, want) {
		t.Fatalf("list lost user lookup failure: %v", err)
	}
}

func TestRegisterSettingsFailureIsPreserved(t *testing.T) {
	want := errors.New("settings unavailable")
	svc, err := service.NewUserService(t.Context(), unavailableUsers{}, failingSettings{err: want})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Register(t.Context(), service.RegisterInput{
		Username: "alice", Email: "alice@example.com", Password: testPassword,
	})
	if !errors.Is(err, want) {
		t.Fatalf("registration lost settings failure: %v", err)
	}
}

func TestCanceledServiceOperationsStopBeforePersistence(t *testing.T) {
	users, err := service.NewUserService(t.Context(), unavailableUsers{}, failingSettings{})
	if err != nil {
		t.Fatal(err)
	}
	tokens, err := service.NewTokenService(
		t.Context(),
		touchFailure{},
		unavailableUsers{},
		func() time.Time { return time.Time{} },
	)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	checks := []struct {
		name string
		run  func() error
	}{
		{name: "register", run: func() error { _, err := users.Register(ctx, service.RegisterInput{}); return err }},
		{name: "credentials", run: func() error { _, err := users.VerifyCredentials(ctx, "", ""); return err }},
		{name: "init admin", run: func() error { _, err := users.InitAdmin(ctx, service.RegisterInput{}); return err }},
		{name: "change password", run: func() error { return users.ChangePassword(ctx, 1, "", "") }},
		{name: "reset password", run: func() error { return users.ResetPassword(ctx, "", "") }},
		{name: "issue", run: func() error {
			_, err := tokens.Issue(ctx, service.TokenSubject{}, service.TokenInput{})
			return err
		}},
		{name: "authenticate", run: func() error { _, err := tokens.Authenticate(ctx, ""); return err }},
		{name: "list", run: func() error { _, err := tokens.List(ctx, 1); return err }},
		{name: "revoke", run: func() error { return tokens.Revoke(ctx, 1, 1) }},
		{name: "revoke all", run: func() error { return tokens.RevokeAll(ctx, 1) }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled operation error=%v", err)
			}
		})
	}
}
