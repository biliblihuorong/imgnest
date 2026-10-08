package service

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"fmt"
	"log/slog"
	mathrand "math/rand/v2"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// Random-link limits. Candidates are cached briefly so anonymous traffic does
// not reach the image table on every request.
const (
	RandomPoolTTL   = 60 * time.Second
	RandomPoolLimit = 5000

	randomPublicIDLength = 10
	randomTokenLength    = 24
	randomTokenAttempts  = 5
	base62Alphabet       = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
)

// RandomPoolInvalidator drops one album's cached redirect candidates.
type RandomPoolInvalidator interface {
	Invalidate(ctx context.Context, albumID uint64) error
}

// RandomPool caches the redirect candidates of one album. The in-memory
// implementation serves a single instance; a shared cache can satisfy the
// same interface when several instances run.
type RandomPool interface {
	RandomPoolInvalidator
	Get(ctx context.Context, albumID uint64) ([]model.RandomCandidate, bool, error)
	Set(ctx context.Context, albumID uint64, items []model.RandomCandidate, ttl time.Duration) error
}

// RandomLinkRepository persists links and reads the images they may serve.
type RandomLinkRepository interface {
	FindByAlbum(ctx context.Context, albumID uint64) (model.RandomLink, error)
	Create(ctx context.Context, link model.RandomLink) (model.RandomLink, error)
	Update(ctx context.Context, albumID uint64, values map[string]any) (model.RandomLink, error)
	DeleteByAlbum(ctx context.Context, albumID uint64) error
	FindByToken(ctx context.Context, token string) (model.RandomLink, model.User, error)
	Candidates(ctx context.Context, albumID uint64, limit int) ([]model.RandomCandidate, error)
}

// PublicIDStore assigns an account its random public handle on first use.
type PublicIDStore interface {
	EnsurePublicID(ctx context.Context, userID uint64, generate func() (string, error)) (string, error)
}

// RandomStorages resolves the backend whose base URL a redirect points at.
type RandomStorages interface {
	Find(ctx context.Context, id uint64) (model.Storage, error)
}

// RandomLinkDependencies are the capabilities random links need.
type RandomLinkDependencies struct {
	Links    RandomLinkRepository
	Albums   AlbumStore
	Users    PublicIDStore
	Storages RandomStorages
	Pool     RandomPool
	Logger   *slog.Logger
	// Intn picks an index in [0, n); nil uses math/rand/v2.
	Intn func(n int) int
}

// RandomLinkView is the owner-facing link; Path is rebuilt on every read and
// the client prefixes its own origin.
type RandomLinkView struct {
	Enabled   bool      `json:"enabled"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"created_at"`
}

// RandomLinkService manages per-album random-image links and resolves them
// into redirect targets.
type RandomLinkService struct{ deps RandomLinkDependencies }

// NewRandomLinkService constructs random-link operations.
func NewRandomLinkService(ctx context.Context, deps RandomLinkDependencies) (*RandomLinkService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct random link service: %w", err)
	}
	if deps.Links == nil || deps.Albums == nil || deps.Users == nil || deps.Storages == nil || deps.Pool == nil || deps.Logger == nil {
		return nil, fmt.Errorf("random link service dependencies: %w", ErrInvalidInput)
	}
	if deps.Intn == nil {
		deps.Intn = mathrand.IntN
	}
	return &RandomLinkService{deps: deps}, nil
}

// Get returns nil when the album has no link.
func (s *RandomLinkService) Get(ctx context.Context, ownerID, albumID uint64) (*RandomLinkView, error) {
	if err := s.owned(ctx, ownerID, albumID); err != nil {
		return nil, fmt.Errorf("get random link: %w", err)
	}
	link, err := s.deps.Links.FindByAlbum(ctx, albumID)
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get random link: %w", err)
	}
	view, err := s.view(ctx, ownerID, link)
	if err != nil {
		return nil, fmt.Errorf("get random link: %w", err)
	}
	return &view, nil
}

// Put creates the album's link on first use and otherwise only switches it on
// or off; the token survives so published URLs keep working.
func (s *RandomLinkService) Put(ctx context.Context, ownerID, albumID uint64, enabled bool) (RandomLinkView, error) {
	if err := s.owned(ctx, ownerID, albumID); err != nil {
		return RandomLinkView{}, fmt.Errorf("put random link: %w", err)
	}
	link, err := s.deps.Links.FindByAlbum(ctx, albumID)
	if errors.Is(err, ErrNotFound) {
		link, err = s.create(ctx, ownerID, albumID, enabled)
	}
	if err != nil {
		return RandomLinkView{}, fmt.Errorf("put random link: %w", err)
	}
	if link.Enabled != enabled {
		if link, err = s.deps.Links.Update(ctx, albumID, map[string]any{"enabled": enabled}); err != nil {
			return RandomLinkView{}, fmt.Errorf("put random link: %w", err)
		}
	}
	view, err := s.view(ctx, ownerID, link)
	if err != nil {
		return RandomLinkView{}, fmt.Errorf("put random link: %w", err)
	}
	return view, nil
}

// Reset replaces the token so every previously shared URL stops resolving.
func (s *RandomLinkService) Reset(ctx context.Context, ownerID, albumID uint64) (RandomLinkView, error) {
	if err := s.owned(ctx, ownerID, albumID); err != nil {
		return RandomLinkView{}, fmt.Errorf("reset random link: %w", err)
	}
	var err error
	for range randomTokenAttempts {
		var token string
		if token, err = randomBase62(randomTokenLength); err != nil {
			return RandomLinkView{}, fmt.Errorf("reset random link: %w", err)
		}
		var link model.RandomLink
		link, err = s.deps.Links.Update(ctx, albumID, map[string]any{"token": token})
		if err == nil {
			view, viewErr := s.view(ctx, ownerID, link)
			if viewErr != nil {
				return RandomLinkView{}, fmt.Errorf("reset random link: %w", viewErr)
			}
			return view, nil
		}
		if !errors.Is(err, model.ErrRandomLinkExists) {
			break
		}
	}
	return RandomLinkView{}, fmt.Errorf("reset random link: %w", err)
}

// Delete removes the album's link; deleting a missing link succeeds.
func (s *RandomLinkService) Delete(ctx context.Context, ownerID, albumID uint64) error {
	if err := s.owned(ctx, ownerID, albumID); err != nil {
		return fmt.Errorf("delete random link: %w", err)
	}
	if err := s.deps.Links.DeleteByAlbum(ctx, albumID); err != nil {
		return fmt.Errorf("delete random link: %w", err)
	}
	s.invalidate(ctx, albumID)
	return nil
}

// Pick returns the redirect target; every lookup miss is ErrNotFound so the
// endpoint reveals nothing about which part of the URL was wrong.
func (s *RandomLinkService) Pick(ctx context.Context, uid, token string, original bool) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", fmt.Errorf("pick random image: %w", err)
	}
	if !ValidRandomSegment(uid, randomPublicIDLength) || !ValidRandomSegment(token, randomTokenLength) {
		return "", ErrNotFound
	}
	link, owner, err := s.deps.Links.FindByToken(ctx, token)
	if err != nil {
		return "", fmt.Errorf("pick random image: %w", err)
	}
	if owner.PublicID == nil || subtle.ConstantTimeCompare([]byte(*owner.PublicID), []byte(uid)) != 1 {
		return "", ErrNotFound
	}
	if !link.Enabled || owner.Status != model.UserStatusEnabled {
		return "", ErrNotFound
	}
	candidates, err := s.candidates(ctx, link.AlbumID)
	if err != nil {
		return "", fmt.Errorf("pick random image: %w", err)
	}
	if len(candidates) == 0 {
		return "", ErrNotFound
	}
	chosen := candidates[s.deps.Intn(len(candidates))]
	backend, err := s.deps.Storages.Find(ctx, chosen.StorageID)
	if err != nil {
		return "", fmt.Errorf("pick random image: %w", err)
	}
	return objectURL(backend.BaseURL, randomObjectKey(chosen, original)), nil
}

// ValidRandomSegment reports whether value is exactly n base62 characters.
func ValidRandomSegment(value string, n int) bool {
	if len(value) != n {
		return false
	}
	for i := range len(value) {
		c := value[i]
		if (c < '0' || c > '9') && (c < 'A' || c > 'Z') && (c < 'a' || c > 'z') {
			return false
		}
	}
	return true
}

func (s *RandomLinkService) owned(ctx context.Context, ownerID, albumID uint64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ownerID == 0 || albumID == 0 {
		return ErrInvalidInput
	}
	_, err := s.deps.Albums.FindOwned(ctx, ownerID, albumID)
	return err
}

// create inserts a link with a fresh token. Losing a race for the same album
// returns the winner's link; a token collision is retried.
func (s *RandomLinkService) create(ctx context.Context, ownerID, albumID uint64, enabled bool) (model.RandomLink, error) {
	var err error
	for range randomTokenAttempts {
		var token string
		if token, err = randomBase62(randomTokenLength); err != nil {
			return model.RandomLink{}, err
		}
		var link model.RandomLink
		link, err = s.deps.Links.Create(ctx, model.RandomLink{UserID: ownerID, AlbumID: albumID, Token: token, Enabled: enabled})
		if err == nil {
			// A reused album ID must never inherit an older album's candidates.
			s.invalidate(ctx, albumID)
			return link, nil
		}
		if !errors.Is(err, model.ErrRandomLinkExists) {
			return model.RandomLink{}, err
		}
		if existing, findErr := s.deps.Links.FindByAlbum(ctx, albumID); findErr == nil {
			return existing, nil
		} else if !errors.Is(findErr, ErrNotFound) {
			return model.RandomLink{}, findErr
		}
	}
	return model.RandomLink{}, err
}

func (s *RandomLinkService) view(ctx context.Context, ownerID uint64, link model.RandomLink) (RandomLinkView, error) {
	publicID, err := s.deps.Users.EnsurePublicID(ctx, ownerID, func() (string, error) { return randomBase62(randomPublicIDLength) })
	if err != nil {
		return RandomLinkView{}, err
	}
	return RandomLinkView{Enabled: link.Enabled, Path: "/random/" + publicID + "/" + link.Token, CreatedAt: link.CreatedAt.UTC()}, nil
}

// candidates reads through the pool. A failing pool only costs a database
// read: redirects must keep working when the cache does not.
func (s *RandomLinkService) candidates(ctx context.Context, albumID uint64) ([]model.RandomCandidate, error) {
	cached, ok, err := s.deps.Pool.Get(ctx, albumID)
	if err != nil {
		s.deps.Logger.WarnContext(ctx, "random pool read failed", "album_id", albumID)
	} else if ok {
		return cached, nil
	}
	fresh, err := s.deps.Links.Candidates(ctx, albumID, RandomPoolLimit)
	if err != nil {
		return nil, err
	}
	if err := s.deps.Pool.Set(ctx, albumID, fresh, RandomPoolTTL); err != nil {
		s.deps.Logger.WarnContext(ctx, "random pool write failed", "album_id", albumID)
	}
	return fresh, nil
}

func (s *RandomLinkService) invalidate(ctx context.Context, albumID uint64) {
	if err := s.deps.Pool.Invalidate(ctx, albumID); err != nil {
		s.deps.Logger.WarnContext(ctx, "random pool invalidation failed", "album_id", albumID)
	}
}

// randomObjectKey prefers WebP unless the original was requested, and falls
// back to whichever version the image actually has.
func randomObjectKey(image model.RandomCandidate, original bool) string {
	if (original && image.HasOriginal) || !image.HasWebP {
		return image.Path + "." + image.Ext
	}
	return image.Path + ".webp"
}

// randomBase62 draws n unbiased base62 characters from crypto/rand.
func randomBase62(n int) (string, error) {
	out := make([]byte, 0, n)
	buf := make([]byte, n*2)
	for len(out) < n {
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("generate random identifier: %w", err)
		}
		for _, b := range buf {
			// 248 is the largest multiple of 62 below 256; higher bytes would bias.
			if b < 248 && len(out) < n {
				out = append(out, base62Alphabet[b%62])
			}
		}
	}
	return string(out), nil
}
