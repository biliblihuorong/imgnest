// Package secret authenticates storage credentials before they reach the database.
package secret

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrInvalid identifies a missing key or an unauthenticated encrypted value.
var ErrInvalid = errors.New("invalid encrypted storage configuration")

// Codec uses the deployment's 32-byte master key and fresh nonces.
type Codec struct{ aead cipher.AEAD }
type envelope struct {
	Version int    `json:"version"`
	Sealed  string `json:"sealed"`
}

// NewCodec accepts a base64-encoded key; an empty key permits local-only installations.
func NewCodec(ctx context.Context, key string) (*Codec, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("create secret codec: %w", err)
	}
	if key == "" {
		return &Codec{}, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return nil, ErrInvalid
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, ErrInvalid
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, ErrInvalid
	}
	return &Codec{aead: aead}, nil
}

// Seal returns a versioned JSON envelope, authenticated against its storage driver.
func (c *Codec) Seal(ctx context.Context, driver string, plaintext []byte) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("encrypt storage configuration: %w", err)
	}
	if c == nil || c.aead == nil || driver == "" || !json.Valid(plaintext) {
		return nil, ErrInvalid
	}
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate storage nonce: %w", err)
	}
	sealed := c.aead.Seal(nonce, nonce, plaintext, []byte("imgnest/storage/v1/"+driver))
	value, err := json.Marshal(envelope{Version: 1, Sealed: base64.StdEncoding.EncodeToString(sealed)})
	if err != nil {
		return nil, ErrInvalid
	}
	return value, nil
}

// Open rejects wrong keys, malformed envelopes, tampering, and driver substitutions.
func (c *Codec) Open(ctx context.Context, driver string, value json.RawMessage) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("decrypt storage configuration: %w", err)
	}
	if c == nil || c.aead == nil || driver == "" {
		return nil, ErrInvalid
	}
	var enc envelope
	if json.Unmarshal(value, &enc) != nil || enc.Version != 1 {
		return nil, ErrInvalid
	}
	sealed, err := base64.StdEncoding.DecodeString(enc.Sealed)
	if err != nil || len(sealed) < c.aead.NonceSize()+c.aead.Overhead() {
		return nil, ErrInvalid
	}
	plain, err := c.aead.Open(nil, sealed[:c.aead.NonceSize()], sealed[c.aead.NonceSize():], []byte("imgnest/storage/v1/"+driver))
	if err != nil || !json.Valid(plain) {
		return nil, ErrInvalid
	}
	return plain, nil
}
