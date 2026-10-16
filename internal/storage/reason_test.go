package storage

import (
	"context"
	"crypto/x509"
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"

	"github.com/aws/smithy-go"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestReasonClassifiesDriverFailures(t *testing.T) {
	api := func(code string) error {
		return s3Error("put object", &smithy.GenericAPIError{Code: code, Message: "secret detail"})
	}
	for want, err := range map[string]error{
		"storage_denied":      api("SignatureDoesNotMatch"),
		"storage_bucket":      api("NoSuchBucket"),
		"storage_busy":        api("SlowDown"),
		"storage_clock":       api("RequestTimeTooSkewed"),
		"storage_unsupported": api("NotImplemented"),
		"storage_tls":         safeError("put", x509.UnknownAuthorityError{}),
		"storage_unreachable": safeError("put", &net.DNSError{Err: "no such host", Name: "s3.example"}),
		"storage_disk_full":   safeError("write", &os.PathError{Op: "write", Path: "/data/x", Err: syscall.ENOSPC}),
		"storage_read_only":   safeError("write", &os.PathError{Op: "open", Path: "/data/x", Err: syscall.EROFS}),
		"storage_permission":  safeError("write", &os.PathError{Op: "open", Path: "/data/x", Err: syscall.EACCES}),
		"storage_timeout":     safeError("put", &net.OpError{Op: "read", Err: timeoutError{}}),
	} {
		if got := Reason(fmt.Errorf("wrapped: %w", err)); got != want {
			t.Fatalf("Reason(%v) = %q, want %q", err, got, want)
		}
	}
	for _, err := range []error{nil, errors.New("other"), context.Canceled, api("Weird")} {
		if got := Reason(err); got != "" {
			t.Fatalf("Reason(%v) = %q, want empty", err, got)
		}
	}
}
