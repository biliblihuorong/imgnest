package service

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestAdminResetUserPasswordHashesAndValidates(t *testing.T) {
	svc, users, _, _, _, _, _ := adminServiceFixture(t, AdminDependencies{})
	if err := svc.ResetUserPassword(t.Context(), 1, 7, "a brand new secret"); err != nil {
		t.Fatal(err)
	}
	hash := users.resetHashes[7]
	if hash == "" || bcrypt.CompareHashAndPassword([]byte(hash), []byte("a brand new secret")) != nil {
		t.Fatalf("stored hash %q does not verify the new password", hash)
	}
	for name, tc := range map[string]struct {
		caller, target uint64
		password       string
		want           error
	}{
		"short password": {1, 7, "short", ErrInvalidInput},
		"long password":  {1, 7, string(make([]byte, 73)), ErrInvalidInput},
		"own account":    {7, 7, "a brand new secret", ErrForbidden},
		"no caller":      {0, 7, "a brand new secret", ErrInvalidInput},
		"no target":      {1, 0, "a brand new secret", ErrInvalidInput},
		"missing target": {1, 99, "a brand new secret", ErrNotFound},
	} {
		if err := svc.ResetUserPassword(t.Context(), tc.caller, tc.target, tc.password); !errors.Is(err, tc.want) {
			t.Fatalf("%s = %v, want %v", name, err, tc.want)
		}
	}
	if err := svc.ResetUserPassword(canceledContext(), 1, 8, "a brand new secret"); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reset = %v", err)
	}
}
