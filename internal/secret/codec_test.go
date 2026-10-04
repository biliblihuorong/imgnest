package secret

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
)

func TestStorageSecrets(t *testing.T) {
	ctx := t.Context()
	key := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 32))
	codec, err := NewCodec(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte(`{"secret_access_key":"private-marker"}`)
	first, err := codec.Seal(ctx, "s3", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	second, err := codec.Seal(ctx, "s3", plaintext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(first, second) || strings.Contains(string(first), "private-marker") {
		t.Fatal("nonce reuse or plaintext persistence")
	}
	out, err := codec.Open(ctx, "s3", first)
	if err != nil || !bytes.Equal(out, plaintext) {
		t.Fatalf("round trip: %v", err)
	}
	if _, err = codec.Open(ctx, "local", first); err == nil {
		t.Fatal("driver binding not authenticated")
	}
	first[len(first)-3] ^= 1
	if _, err = codec.Open(ctx, "s3", first); err == nil {
		t.Fatal("tamper accepted")
	}
	wrong, _ := NewCodec(ctx, base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32)))
	if _, err = wrong.Open(ctx, "s3", second); err == nil || strings.Contains(err.Error(), "private-marker") {
		t.Fatal("wrong key or secret in error")
	}
	empty, err := NewCodec(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = empty.Seal(ctx, "s3", plaintext); err == nil {
		t.Fatal("missing key accepted")
	}
	if _, err = NewCodec(ctx, "secret-marker"); err == nil || strings.Contains(err.Error(), "secret-marker") {
		t.Fatal("invalid key accepted or leaked")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err = codec.Open(canceled, "s3", second); err == nil {
		t.Fatal("canceled context ignored")
	}
}
