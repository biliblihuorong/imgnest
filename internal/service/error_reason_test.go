package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"syscall"
	"testing"
)

func TestErrorReasonNamesInputAndStorageCauses(t *testing.T) {
	full := &os.PathError{Op: "write", Path: "/data/objects/x", Err: syscall.ENOSPC}
	for want, err := range map[string]error{
		"password_length":   fmt.Errorf("register: %w", errPasswordLength),
		"email_invalid":     fmt.Errorf("email: %w", errEmailInvalid),
		"storage_disk_full": storageError(context.Background(), full),
		"":                  ErrInvalidInput,
	} {
		if got := ErrorReason(err); got != want {
			t.Fatalf("ErrorReason(%v) = %q, want %q", err, got, want)
		}
	}
	if !errors.Is(errPasswordLength, ErrInvalidInput) {
		t.Fatal("reason hides the domain error")
	}
	wrapped := storageError(context.Background(), full)
	if !errors.Is(wrapped, ErrStorage) || wrapped.Error() != ErrStorage.Error() {
		t.Fatalf("storage failure = %v", wrapped)
	}
	if ErrorReason(storageError(context.Background(), errors.New("x"))) != "" {
		t.Fatal("unknown storage cause got a reason")
	}
}

func TestPasswordRulesCarryTheirReason(t *testing.T) {
	if _, err := normalizeUsername("ab"); ErrorReason(err) != "username_length" {
		t.Fatalf("short username reason = %v", err)
	}
	if _, err := normalizeEmail("not-an-email"); ErrorReason(err) != "email_invalid" {
		t.Fatalf("bad email reason = %v", err)
	}
	if _, err := normalizeImageName("a/b"); ErrorReason(err) != "image_name_invalid" {
		t.Fatalf("bad image name reason = %v", err)
	}
}
