package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type listedPolicyRepo struct {
	*uploadPolicy
	rows []model.Policy
	err  error
}

func (r listedPolicyRepo) GroupPolicies(context.Context, uint64) ([]model.Policy, error) {
	return r.rows, r.err
}

func TestListPoliciesReturnsGroupBoundEnabledRules(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	svc.deps.Policies = listedPolicyRepo{uploadPolicy: policy, rows: []model.Policy{
		{ID: 3, Name: "wide", Enabled: true}, {ID: 7, Name: "first", Enabled: true},
	}}
	rules, err := svc.ListPolicies(t.Context(), subject)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 || rules[0].ID != 3 || rules[0].Name != "wide" || rules[1].ID != 7 || rules[1].Name != "first" {
		t.Fatalf("rules %+v", rules)
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	for index, rule := range decoded {
		if len(rule) != 2 {
			t.Fatalf("rule %d exposes %d fields: %s", index, len(rule), raw)
		}
	}
}

func TestListPoliciesEmptyIsArray(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	svc.deps.Policies = listedPolicyRepo{uploadPolicy: policy, rows: []model.Policy{}}
	rules, err := svc.ListPolicies(t.Context(), subject)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]" {
		t.Fatalf("empty rules %s", raw)
	}
}

func TestListPoliciesRejectsInvalidSubject(t *testing.T) {
	svc, _, policy, _, _ := uploadFixture(t, "png")
	svc.deps.Policies = listedPolicyRepo{uploadPolicy: policy}
	if _, err := svc.ListPolicies(t.Context(), TokenSubject{}); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("zero subject accepted: %v", err)
	}
}

func TestListPoliciesRejectsDisabledActor(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	svc.deps.Policies = listedPolicyRepo{uploadPolicy: policy}
	svc.deps.Users = uploadUserRepo{user: model.User{ID: 1, GroupID: 1, PasswordHash: "verified", Status: model.UserStatusDisabled, Role: model.UserRoleUser}}
	if _, err := svc.ListPolicies(t.Context(), subject); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("disabled actor accepted: %v", err)
	}
}

func TestListPoliciesWrapsRepoFailure(t *testing.T) {
	svc, _, policy, _, subject := uploadFixture(t, "png")
	svc.deps.Policies = listedPolicyRepo{uploadPolicy: policy, err: model.ErrStorage}
	if _, err := svc.ListPolicies(t.Context(), subject); !errors.Is(err, ErrStorage) {
		t.Fatalf("repo failure error=%v", err)
	}
}
