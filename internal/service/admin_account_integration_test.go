package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
)

func managedValue[T any](value T) *T { return &value }

func TestVerifiedCredentialProofCannotIssueAfterAdminIdentityEdit(t *testing.T) {
	for _, field := range []string{"username", "email", "role", "group"} {
		t.Run(field, func(t *testing.T) {
			forDatabases(t, func(t *testing.T, f *authFixture) {
				owner, err := f.service.InitAdmin(t.Context(), service.RegisterInput{Username: "owner", Email: "owner@example.com", Password: testPassword})
				if err != nil {
					t.Fatal(err)
				}
				target := registerUser(t, f, "target")
				proof, err := f.service.VerifyCredentials(t.Context(), target.Email, testPassword)
				if err != nil {
					t.Fatal(err)
				}
				admin, err := repo.NewAdminRepository(t.Context(), f.db)
				if err != nil {
					t.Fatal(err)
				}
				patch := model.UserChanges{}
				switch field {
				case "username":
					patch.Username = managedValue("renamed")
				case "email":
					patch.Email = managedValue("renamed@example.com")
				case "role":
					patch.Role = managedValue(model.UserRoleAdmin)
				case "group":
					group, err := admin.CreateGroup(t.Context(), model.Group{Name: "Another"}, nil)
					if err != nil {
						t.Fatal(err)
					}
					patch.GroupID = &group.ID
				}
				updated, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, patch)
				if err != nil {
					t.Fatal(err)
				}
				tokens, err := service.NewTokenService(t.Context(), f.tokens, f.users, f.settings, time.Now)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tokens.Issue(t.Context(), proof.Subject, service.TokenInput{Name: "stale", Kind: "web", Abilities: []string{"*"}}); !errors.Is(err, service.ErrInvalidCredentials) {
					t.Fatalf("stale proof accepted after %s change: %v", field, err)
				}
				fresh, err := f.service.VerifyCredentials(t.Context(), updated.Email, testPassword)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = tokens.Issue(t.Context(), fresh.Subject, service.TokenInput{Name: "fresh", Kind: "web", Abilities: []string{"*"}}); err != nil {
					t.Fatalf("fresh credentials rejected: %v", err)
				}
			})
		})
	}
}

func TestAdminEditRacesWithSessionIssuance(t *testing.T) {
	forDatabases(t, func(t *testing.T, f *authFixture) {
		owner, err := f.service.InitAdmin(t.Context(), service.RegisterInput{Username: "owner", Email: "owner@example.com", Password: testPassword})
		if err != nil {
			t.Fatal(err)
		}
		target := registerUser(t, f, "target")
		proof, err := f.service.VerifyCredentials(t.Context(), target.Email, testPassword)
		if err != nil {
			t.Fatal(err)
		}
		admin, err := repo.NewAdminRepository(t.Context(), f.db)
		if err != nil {
			t.Fatal(err)
		}
		tokens, err := service.NewTokenService(t.Context(), f.tokens, f.users, f.settings, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		start := make(chan struct{})
		editResult := make(chan error, 1)
		issueResult := make(chan error, 1)
		go func() {
			<-start
			_, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, model.UserChanges{Email: managedValue("new@example.com")})
			editResult <- err
		}()
		go func() {
			<-start
			_, err := tokens.Issue(t.Context(), proof.Subject, service.TokenInput{Name: "racing", Kind: "web", Abilities: []string{"*"}})
			issueResult <- err
		}()
		close(start)
		if err := <-editResult; err != nil {
			t.Fatal(err)
		}
		if err := <-issueResult; err != nil && !errors.Is(err, service.ErrInvalidCredentials) {
			t.Fatal(err)
		}
		remaining, err := f.tokens.ListTokens(t.Context(), target.ID)
		if err != nil || len(remaining) != 0 {
			t.Fatalf("revocation race left %d tokens: %v", len(remaining), err)
		}
	})
}

func TestVerifiedProofStaysRevokedAfterAccountStateRoundTrip(t *testing.T) {
	for _, field := range []string{"username", "email", "role", "status", "group"} {
		t.Run(field, func(t *testing.T) {
			forDatabases(t, func(t *testing.T, f *authFixture) {
				owner, err := f.service.InitAdmin(t.Context(), service.RegisterInput{Username: "owner", Email: "owner@example.com", Password: testPassword})
				if err != nil {
					t.Fatal(err)
				}
				target := registerUser(t, f, "target")
				proof, err := f.service.VerifyCredentials(t.Context(), target.Email, testPassword)
				if err != nil {
					t.Fatal(err)
				}
				admin, err := repo.NewAdminRepository(t.Context(), f.db)
				if err != nil {
					t.Fatal(err)
				}
				var change, restore model.UserChanges
				switch field {
				case "username":
					change.Username = managedValue("changed")
					restore.Username = &target.Username
				case "email":
					change.Email = managedValue("changed@example.com")
					restore.Email = &target.Email
				case "role":
					change.Role = managedValue(model.UserRoleAdmin)
					restore.Role = &target.Role
				case "status":
					change.Status = managedValue(model.UserStatusDisabled)
					restore.Status = &target.Status
				case "group":
					group, err := admin.CreateGroup(t.Context(), model.Group{Name: "Other"}, nil)
					if err != nil {
						t.Fatal(err)
					}
					change.GroupID = &group.ID
					restore.GroupID = &target.GroupID
				}
				if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, change); err != nil {
					t.Fatal(err)
				}
				if _, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, restore); err != nil {
					t.Fatal(err)
				}
				tokens, err := service.NewTokenService(t.Context(), f.tokens, f.users, f.settings, time.Now)
				if err != nil {
					t.Fatal(err)
				}
				issued, err := tokens.Issue(t.Context(), proof.Subject, service.TokenInput{Name: "stale", Kind: "web", Abilities: []string{"*"}})
				if !errors.Is(err, service.ErrInvalidCredentials) {
					t.Fatalf("stale pre-revocation proof was accepted after %s roundtrip: issued token ID=%d error=%v", field, issued.Info.ID, err)
				}
			})
		})
	}
}

func TestDisplayNameAndNoopEditsPreserveVerifiedProof(t *testing.T) {
	forDatabases(t, func(t *testing.T, f *authFixture) {
		owner, err := f.service.InitAdmin(t.Context(), service.RegisterInput{Username: "owner", Email: "owner@example.com", Password: testPassword})
		if err != nil {
			t.Fatal(err)
		}
		target := registerUser(t, f, "target")
		proof, err := f.service.VerifyCredentials(t.Context(), target.Email, testPassword)
		if err != nil {
			t.Fatal(err)
		}
		admin, err := repo.NewAdminRepository(t.Context(), f.db)
		if err != nil {
			t.Fatal(err)
		}
		for _, patch := range []model.UserChanges{
			{}, {DisplayName: managedValue("Nickname")},
			{Username: &target.Username, Email: &target.Email, Role: &target.Role, Status: &target.Status, GroupID: &target.GroupID},
		} {
			updated, err := admin.UpdateAdminUser(t.Context(), owner.ID, target.ID, patch)
			if err != nil {
				t.Fatal(err)
			}
			if updated.AuthVersion != 0 {
				t.Fatalf("non-security edit advanced epoch: %d", updated.AuthVersion)
			}
		}
		tokens, err := service.NewTokenService(t.Context(), f.tokens, f.users, f.settings, time.Now)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tokens.Issue(t.Context(), proof.Subject, service.TokenInput{Name: "preserved", Kind: "web", Abilities: []string{"*"}}); err != nil {
			t.Fatalf("nickname/no-op revoked verified proof: %v", err)
		}
	})
}
