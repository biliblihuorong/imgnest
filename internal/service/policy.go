package service

import (
	"context"
	"fmt"
)

// PolicySummary is one upload-page entry for a rule the caller may use.
type PolicySummary struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// ListPolicies returns the enabled rules bound to the caller's group whose
// storage backend is enabled, ordered by ID. It reuses the upload-time grant
// semantics, so every listed rule is acceptable to Preflight. An empty list is
// serialized as [] rather than null.
func (s *ImageService) ListPolicies(ctx context.Context, subject TokenSubject) ([]PolicySummary, error) {
	user, err := s.actor(ctx, subject)
	if err != nil {
		return nil, err
	}
	rules, err := s.deps.Policies.GroupPolicies(ctx, user.GroupID)
	if err != nil {
		return nil, fmt.Errorf("list permitted policies: %w", err)
	}
	summaries := make([]PolicySummary, 0, len(rules))
	for _, rule := range rules {
		summaries = append(summaries, PolicySummary{ID: rule.ID, Name: rule.Name})
	}
	return summaries, nil
}
