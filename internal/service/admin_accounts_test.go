package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"golang.org/x/crypto/bcrypt"
)

func adminAccountValue[T any](value T) *T { return &value }

func TestAdminCreateUserNormalizesHashesAndDecorates(t *testing.T) {
	UseProductionPasswordCost(t)
	svc, users, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	view, err := svc.CreateUser(t.Context(), 7, AdminUserInput{
		Username: "  managed  ", Email: "  MANAGED@EXAMPLE.COM ", Password: "initial-password",
		DisplayName: "  管理账号  ", Role: model.UserRoleAdmin, Status: model.UserStatusDisabled, GroupID: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := users.users[view.ID]
	if stored.Username != "managed" || stored.Email != "managed@example.com" || stored.DisplayName != "管理账号" ||
		stored.Role != model.UserRoleAdmin || stored.Status != model.UserStatusDisabled || stored.GroupID != 3 {
		t.Fatalf("created=%+v", view)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("initial-password")); err != nil {
		t.Fatal("initial password was not hashed")
	}
	if cost, err := bcrypt.Cost([]byte(stored.PasswordHash)); err != nil || cost != productionPasswordCost {
		t.Fatalf("cost=%d err=%v", cost, err)
	}
	if view.AvatarProvider != model.DefaultAvatarProvider || view.AvatarURL == nil {
		t.Fatalf("avatar decoration missing: %+v", view)
	}
}

func TestAdminCreateUserValidatesInputs(t *testing.T) {
	cases := []struct {
		name   string
		change func(*AdminUserInput)
	}{
		{"short username", func(v *AdminUserInput) { v.Username = "ab" }},
		{"long username", func(v *AdminUserInput) { v.Username = strings.Repeat("界", 65) }},
		{"invalid email", func(v *AdminUserInput) { v.Email = "Name <name@example.com>" }},
		{"short password", func(v *AdminUserInput) { v.Password = "short" }},
		{"byte password limit", func(v *AdminUserInput) { v.Password = strings.Repeat("界", 25) }},
		{"display control", func(v *AdminUserInput) { v.DisplayName = "bad\x00name" }},
		{"display length", func(v *AdminUserInput) { v.DisplayName = strings.Repeat("界", 65) }},
		{"role", func(v *AdminUserInput) { v.Role = "superadmin" }},
		{"status", func(v *AdminUserInput) { v.Status = "pending" }},
		{"zero group", func(v *AdminUserInput) { v.GroupID = 0 }},
		{"guest group", func(v *AdminUserInput) { v.GroupID = 2 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			svc, users, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
			input := AdminUserInput{Username: "managed", Email: "managed@example.com", Password: "initial-password", Role: "user", Status: "enabled", GroupID: 1}
			test.change(&input)
			if _, err := svc.CreateUser(t.Context(), 7, input); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("invalid input=%v", err)
			}
			if len(users.users) != 2 {
				t.Fatal("invalid input wrote account")
			}
		})
	}
}

func TestAdminPatchUserNormalizesIdentityAndDisplayName(t *testing.T) {
	svc, users, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	view, err := svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{
		Username: adminAccountValue("  renamed  "), Email: adminAccountValue("  NEW@EXAMPLE.COM "),
		DisplayName: adminAccountValue("  新名字  "), Role: adminAccountValue("admin"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.Username != "renamed" || view.Email != "new@example.com" || view.DisplayName != "新名字" || view.Role != "admin" {
		t.Fatalf("patch=%+v", view)
	}
	if users.users[8].PasswordHash != "" {
		t.Fatal("patch changed password")
	}
	for _, patch := range []AdminUserPatch{
		{Username: adminAccountValue("ab")}, {Email: adminAccountValue("bad")}, {Role: adminAccountValue("root")},
		{DisplayName: adminAccountValue("bad\x00name")},
	} {
		if _, err := svc.PatchUser(t.Context(), 7, 8, patch); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid patch=%v", err)
		}
	}
	view, err = svc.PatchUser(t.Context(), 7, 8, AdminUserPatch{DisplayName: adminAccountValue("  ")})
	if err != nil || view.DisplayName != "" {
		t.Fatalf("clear name=%+v %v", view, err)
	}
}
