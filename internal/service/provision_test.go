package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/pathtpl"
	"github.com/biliblihuorong/imgnest/internal/secret"
)

type provisionRepo struct {
	StorageRepository
	rows []model.Storage
}

func (r *provisionRepo) Create(_ context.Context, value model.Storage) (model.Storage, error) {
	value.ID = uint64(len(r.rows) + 1)
	r.rows = append(r.rows, value)
	return value, nil
}
func (r *provisionRepo) Find(_ context.Context, id uint64) (model.Storage, error) {
	return r.rows[id-1], nil
}
func TestProvisionEncryptedConfigAndPolicyValidation(t *testing.T) {
	svc, _, policy, local, _ := uploadFixture(t, "png")
	_ = svc
	codec, err := secret.NewCodec(t.Context(), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	stores := &provisionRepo{}
	setup, err := NewProvisionService(t.Context(), stores, policy, uploadDriverProvider{driver: local}, codec, TemplateValidatorFunc(pathtpl.Validate))
	if err != nil {
		t.Fatal(err)
	}
	view, err := setup.CreateStorage(t.Context(), StorageInput{Name: "test", Driver: "s3", BaseURL: "https://images.test", Config: json.RawMessage(`{"secret_access_key":"private-marker"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(stores.rows[0].Config), "private-marker") {
		t.Fatal("credential stored plaintext")
	}
	raw, _ := json.Marshal(view)
	if strings.Contains(string(raw), "config") || strings.Contains(string(raw), "private-marker") {
		t.Fatal("credential returned publicly")
	}
	plain, err := codec.Open(t.Context(), "s3", stores.rows[0].Config)
	if err != nil || !strings.Contains(string(plain), "private-marker") {
		t.Fatal("credential cannot be recovered by key")
	}
	valid, err := DefaultPolicy(t.Context(), view.ID, "default")
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*model.Policy){func(p *model.Policy) { p.NameTpl = "{unknown}" }, func(p *model.Policy) { p.WebPQuality = 101 }, func(p *model.Policy) { p.HEIFMode = "keep"; p.ScrubMode = "gps" }, func(p *model.Policy) { p.WebPMode = "invalid" }, func(p *model.Policy) { p.OnConflict = "overwrite" }} {
		invalid := valid
		change(&invalid)
		if _, err = setup.CreatePolicy(t.Context(), invalid, 1, true); err == nil {
			t.Fatal("invalid policy accepted")
		}
	}
}

// A create request without config follows the optional OpenAPI field: it is
// stored as an empty object instead of being rejected as invalid JSON.
func TestProvisionAbsentConfigBecomesEmptyObject(t *testing.T) {
	_, _, policy, local, _ := uploadFixture(t, "png")
	codec, err := secret.NewCodec(t.Context(), base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{2}, 32)))
	if err != nil {
		t.Fatal(err)
	}
	stores := &provisionRepo{}
	setup, err := NewProvisionService(t.Context(), stores, policy, uploadDriverProvider{driver: local}, codec, TemplateValidatorFunc(pathtpl.Validate))
	if err != nil {
		t.Fatal(err)
	}
	view, err := setup.CreateStorage(t.Context(), StorageInput{
		Name: "local", Driver: "local", BaseURL: "http://images.test",
	})
	if err != nil {
		t.Fatalf("absent config rejected: %v", err)
	}
	if string(stores.rows[0].Config) != "{}" {
		t.Fatalf("stored config=%s, want {}", stores.rows[0].Config)
	}
	if view.ID != stores.rows[0].ID {
		t.Fatal("view does not match the persisted storage")
	}
}
