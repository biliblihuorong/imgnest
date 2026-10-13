package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
)

// A completed account edit between lookup and insert must invalidate a new
// email-based identity link, even if token issuance would reject the old proof.
type externalLookupInterleave struct {
	service.UserRepository
	change func()
}

func (r *externalLookupInterleave) FindUserByEmail(ctx context.Context, email string) (model.User, error) {
	user, err := r.UserRepository.FindUserByEmail(ctx, email)
	if err == nil && r.change != nil {
		change := r.change
		r.change = nil
		change()
	}
	return user, err
}

func TestExternalSignInDoesNotLinkAfterAccountEdit(t *testing.T) {
	for _, edit := range []string{"promotion", "email", "disabled", "password", "authorization-roundtrip"} {
		t.Run(edit, func(t *testing.T) {
			forExternalDatabases(t, func(t *testing.T, f *authFixture) {
				root, err := f.service.InitAdmin(t.Context(), service.RegisterInput{Username: "root", Email: "root@example.com", Password: testPassword})
				if err != nil {
					t.Fatal(err)
				}
				target := registerUser(t, f, "alice")
				admin, err := repo.NewAdminRepository(t.Context(), f.db)
				if err != nil {
					t.Fatal(err)
				}
				interleaved := &externalLookupInterleave{UserRepository: f.users, change: func() {
					role, email, status := model.UserRoleAdmin, "new-owner@example.com", model.UserStatusDisabled
					var patch model.UserChanges
					switch edit {
					case "promotion", "authorization-roundtrip":
						patch.Role = &role
					case "email":
						patch.Email = &email
					case "disabled":
						patch.Status = &status
					case "password":
						if err := f.service.ChangePassword(t.Context(), target.ID, testPassword, "replacement-password-123"); err != nil {
							t.Fatal(err)
						}
						return
					}
					if _, err := admin.UpdateAdminUser(t.Context(), root.ID, target.ID, patch); err != nil {
						t.Fatal(err)
					}
					if edit == "authorization-roundtrip" {
						role = model.UserRoleUser
						if _, err := admin.UpdateAdminUser(t.Context(), root.ID, target.ID, model.UserChanges{Role: &role}); err != nil {
							t.Fatal(err)
						}
					}
				}}
				users, err := service.NewUserService(t.Context(), interleaved, f.settings)
				if err != nil {
					t.Fatal(err)
				}
				users.UseIdentities(f.users)
				in := githubIdentity("pending-link", "alice@example.com")
				in.LinkByEmail = true
				_, signInErr := users.SignInExternal(t.Context(), in)
				if _, err := f.users.FindUserByIdentity(t.Context(), in.Provider, in.Subject); !errors.Is(err, service.ErrNotFound) {
					t.Fatalf("account edit must not leave a usable identity: %v", err)
				}
				if !errors.Is(signInErr, service.ErrIdentityNotLinked) {
					t.Fatalf("sign-in after completed %s = %v, want not linked", edit, signInErr)
				}
			})
		})
	}
}

func externalFixture(t *testing.T, driver string) *authFixture {
	t.Helper()
	fixture := newAuthFixture(t, driver)
	fixture.service.UseIdentities(fixture.users)
	return fixture
}

func forExternalDatabases(t *testing.T, run func(*testing.T, *authFixture)) {
	t.Helper()
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) { run(t, externalFixture(t, driver)) })
	}
}

func githubIdentity(subject, email string) service.ExternalSignIn {
	return service.ExternalSignIn{Provider: "sso:github", Subject: subject, Email: email, EmailVerified: true, UsernameHint: "octo cat!"}
}

func TestExternalSignInCreatesAndReusesAccount(t *testing.T) {
	forExternalDatabases(t, func(t *testing.T, f *authFixture) {
		enableRegistration(t, f)
		first, err := f.service.SignInExternal(t.Context(), githubIdentity("1001", "Octo@Example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if first.User.Username != "octocat" || first.User.Email != "octo@example.com" || first.User.Role != model.UserRoleUser {
			t.Fatalf("created %+v", first.User)
		}
		// Later sign-ins match on the subject alone, even if the email changed.
		again, err := f.service.SignInExternal(t.Context(), service.ExternalSignIn{Provider: "sso:github", Subject: "1001"})
		if err != nil || again.User.ID != first.User.ID {
			t.Fatalf("second sign-in = %+v, %v", again.User, err)
		}
		// The proof issues a normal web token.
		tokens, err := service.NewTokenService(t.Context(), f.tokens, f.users, f.settings, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tokens.Issue(t.Context(), again.Subject, service.TokenInput{Name: "web", Kind: "web", Abilities: []string{"*"}}); err != nil {
			t.Fatalf("issue token: %v", err)
		}
		// The same subject at another provider is a different identity.
		if _, err := f.service.SignInExternal(t.Context(), service.ExternalSignIn{Provider: "sso:google", Subject: "1001"}); !errors.Is(err, service.ErrIdentityEmailRequired) {
			t.Fatalf("other provider err = %v", err)
		}
	})
}

func TestExternalSignInRespectsRegistrationAndEmail(t *testing.T) {
	forExternalDatabases(t, func(t *testing.T, f *authFixture) {
		if _, err := f.service.SignInExternal(t.Context(), githubIdentity("1", "a@example.com")); !errors.Is(err, service.ErrIdentityNotLinked) {
			t.Fatalf("registration closed err = %v", err)
		}
		enableRegistration(t, f)
		unverified := githubIdentity("2", "b@example.com")
		unverified.EmailVerified = false
		if _, err := f.service.SignInExternal(t.Context(), unverified); !errors.Is(err, service.ErrIdentityEmailRequired) {
			t.Fatalf("unverified email err = %v", err)
		}
		if _, err := f.service.SignInExternal(t.Context(), service.ExternalSignIn{Provider: "", Subject: "3"}); !errors.Is(err, service.ErrInvalidInput) {
			t.Fatalf("empty provider err = %v", err)
		}
	})
}

func TestExternalSignInLinksByEmailOnlyWhenAllowed(t *testing.T) {
	forExternalDatabases(t, func(t *testing.T, f *authFixture) {
		existing := registerUser(t, f, "alice")
		in := githubIdentity("42", "alice@example.com")
		if _, err := f.service.SignInExternal(t.Context(), in); !errors.Is(err, service.ErrIdentityNotLinked) {
			t.Fatalf("link without permission err = %v", err)
		}
		in.LinkByEmail = true
		in.EmailVerified = false
		if _, err := f.service.SignInExternal(t.Context(), in); err == nil {
			t.Fatal("an unverified email must not link")
		}
		in.EmailVerified = true
		linked, err := f.service.SignInExternal(t.Context(), in)
		if err != nil || linked.User.ID != existing.ID {
			t.Fatalf("link = %+v, %v", linked.User, err)
		}
	})
}

func TestExternalSignInNeverLinksAdministrators(t *testing.T) {
	forExternalDatabases(t, func(t *testing.T, f *authFixture) {
		if _, err := f.service.InitAdmin(t.Context(), service.RegisterInput{Username: "root", Email: "root@example.com", Password: testPassword}); err != nil {
			t.Fatal(err)
		}
		enableRegistration(t, f)
		in := githubIdentity("7", "root@example.com")
		in.LinkByEmail = true
		if _, err := f.service.SignInExternal(t.Context(), in); !errors.Is(err, service.ErrIdentityNotLinked) {
			t.Fatalf("admin link err = %v", err)
		}
	})
}

func TestExternalSignInRejectsDisabledAccount(t *testing.T) {
	forExternalDatabases(t, func(t *testing.T, f *authFixture) {
		enableRegistration(t, f)
		created, err := f.service.SignInExternal(t.Context(), githubIdentity("9", "dis@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if err := f.db.Model(&model.User{}).Where("id = ?", created.User.ID).Update("status", model.UserStatusDisabled).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.SignInExternal(t.Context(), githubIdentity("9", "")); !errors.Is(err, service.ErrForbidden) {
			t.Fatalf("disabled err = %v", err)
		}
	})
}

func TestExternalSignInPicksAFreeUsername(t *testing.T) {
	forExternalDatabases(t, func(t *testing.T, f *authFixture) {
		registerUser(t, f, "octocat")
		created, err := f.service.SignInExternal(t.Context(), githubIdentity("55", "other@example.com"))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(created.User.Username, "octocat-") {
			t.Fatalf("username = %q", created.User.Username)
		}
		short := githubIdentity("56", "xy@example.com")
		short.UsernameHint = "@"
		created, err = f.service.SignInExternal(t.Context(), short)
		if err != nil || !strings.HasPrefix(created.User.Username, "user-") {
			t.Fatalf("short hint = %q, %v", created.User.Username, err)
		}
	})
}

func TestExternalSignInDisabledWithoutIdentities(t *testing.T) {
	f := newAuthFixture(t, "sqlite")
	enableRegistration(t, f)
	if _, err := f.service.SignInExternal(t.Context(), githubIdentity("1", "a@example.com")); !errors.Is(err, service.ErrIdentityNotLinked) {
		t.Fatalf("err = %v", err)
	}
}
