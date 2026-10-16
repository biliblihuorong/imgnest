package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type renameImagesRepo struct {
	albumImagesRepo
	renamedKey  string
	renamedName string
	renameErr   error
}

func (r *renameImagesRepo) SetName(_ context.Context, key, name string, _ model.TokenGrant) error {
	r.renamedKey, r.renamedName = key, name
	return r.renameErr
}

func TestImageRenameChangesOnlyTheDisplayName(t *testing.T) {
	svc, base, _, subject := albumImageFixture(t)
	images := &renameImagesRepo{albumImagesRepo: *base}
	svc.deps.Images = images
	images.image = activeImage(4)

	view, err := svc.Rename(t.Context(), subject, 4, "  holiday 2026.jpg  ")
	if err != nil {
		t.Fatal(err)
	}
	if images.renamedKey != "image-key" || images.renamedName != "holiday 2026.jpg" {
		t.Fatalf("repository rename = key %q name %q", images.renamedKey, images.renamedName)
	}
	if view.Name != "holiday 2026.jpg" || view.Key != "image-key" {
		t.Fatalf("renamed view = %+v", view)
	}

	for _, name := range []string{"", "   ", "a/b.jpg", `a\b.jpg`, "bad\x00name", strings.Repeat("名", 86), "\xff"} {
		images.renamedName = ""
		if _, err := svc.Rename(t.Context(), subject, 4, name); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("name %q = %v, want invalid input", name, err)
		}
		if images.renamedName != "" {
			t.Fatalf("name %q reached the repository", name)
		}
	}

	cases := []struct {
		name  string
		image model.Image
		find  error
		want  error
	}{
		{"missing image", model.Image{}, ErrNotFound, ErrNotFound},
		{"foreign image", model.Image{ID: 4, UserID: 2, State: model.ImageStateActive}, nil, ErrForbidden},
		{"trashed image", model.Image{ID: 4, UserID: 1, State: model.ImageStateTrash}, nil, ErrInvalidInput},
	}
	for _, tc := range cases {
		images.image, images.findErr = tc.image, tc.find
		if _, err := svc.Rename(t.Context(), subject, 4, "x.jpg"); !errors.Is(err, tc.want) {
			t.Fatalf("%s = %v, want %v", tc.name, err, tc.want)
		}
	}
	images.image, images.findErr, images.renameErr = activeImage(4), nil, ErrImageBusy
	if _, err := svc.Rename(t.Context(), subject, 4, "x.jpg"); !errors.Is(err, ErrImageBusy) {
		t.Fatalf("repository error swallowed: %v", err)
	}
}
