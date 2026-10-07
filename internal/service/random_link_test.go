package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

type candidateCall struct {
	albumID uint64
	limit   int
}

// randomLinkStore is an in-memory RandomLinkRepository keyed by album.
type randomLinkStore struct {
	links          map[uint64]model.RandomLink
	owners         map[uint64]model.User
	candidates     []model.RandomCandidate
	candidateCalls []candidateCall
	createErrs     []error
	createCalls    int
	touched        bool
}

func newRandomLinkStore() *randomLinkStore {
	return &randomLinkStore{links: map[uint64]model.RandomLink{}, owners: map[uint64]model.User{}}
}

func (s *randomLinkStore) FindByAlbum(_ context.Context, albumID uint64) (model.RandomLink, error) {
	s.touched = true
	link, ok := s.links[albumID]
	if !ok {
		return model.RandomLink{}, model.ErrNotFound
	}
	return link, nil
}

func (s *randomLinkStore) Create(_ context.Context, link model.RandomLink) (model.RandomLink, error) {
	s.touched = true
	s.createCalls++
	if len(s.createErrs) > 0 {
		err := s.createErrs[0]
		s.createErrs = s.createErrs[1:]
		if err != nil {
			return model.RandomLink{}, err
		}
	}
	link.ID = uint64(len(s.links) + 1)
	link.CreatedAt = time.Date(2026, 10, 7, 8, 0, 0, 0, time.UTC)
	s.links[link.AlbumID] = link
	return link, nil
}

func (s *randomLinkStore) Update(_ context.Context, albumID uint64, values map[string]any) (model.RandomLink, error) {
	s.touched = true
	link, ok := s.links[albumID]
	if !ok {
		return model.RandomLink{}, model.ErrNotFound
	}
	if enabled, ok := values["enabled"].(bool); ok {
		link.Enabled = enabled
	}
	if token, ok := values["token"].(string); ok {
		link.Token = token
	}
	s.links[albumID] = link
	return link, nil
}

func (s *randomLinkStore) DeleteByAlbum(_ context.Context, albumID uint64) error {
	s.touched = true
	delete(s.links, albumID)
	return nil
}

func (s *randomLinkStore) FindByToken(_ context.Context, token string) (model.RandomLink, model.User, error) {
	s.touched = true
	for _, link := range s.links {
		if link.Token == token {
			return link, s.owners[link.UserID], nil
		}
	}
	return model.RandomLink{}, model.User{}, model.ErrNotFound
}

func (s *randomLinkStore) Candidates(_ context.Context, albumID uint64, limit int) ([]model.RandomCandidate, error) {
	s.candidateCalls = append(s.candidateCalls, candidateCall{albumID: albumID, limit: limit})
	return s.candidates, nil
}

type randomAlbumStub struct{ err error }

func (s randomAlbumStub) FindOwned(_ context.Context, ownerID, albumID uint64) (model.Album, error) {
	return model.Album{ID: albumID, UserID: ownerID}, s.err
}

type publicIDStub struct{ value string }

func (s publicIDStub) EnsurePublicID(context.Context, uint64, func() (string, error)) (string, error) {
	return s.value, nil
}

type randomStorageStub struct {
	base string
	err  error
}

func (s randomStorageStub) Find(_ context.Context, id uint64) (model.Storage, error) {
	return model.Storage{ID: id, BaseURL: s.base}, s.err
}

type randomPoolStub struct {
	items       map[uint64][]model.RandomCandidate
	setTTL      time.Duration
	sets        int
	invalidated []uint64
	getErr      error
	setErr      error
}

func (p *randomPoolStub) Get(_ context.Context, albumID uint64) ([]model.RandomCandidate, bool, error) {
	if p.getErr != nil {
		return nil, false, p.getErr
	}
	items, ok := p.items[albumID]
	return items, ok, nil
}

func (p *randomPoolStub) Set(_ context.Context, albumID uint64, items []model.RandomCandidate, ttl time.Duration) error {
	p.sets++
	p.setTTL = ttl
	if p.setErr != nil {
		return p.setErr
	}
	if p.items == nil {
		p.items = map[uint64][]model.RandomCandidate{}
	}
	p.items[albumID] = items
	return nil
}

func (p *randomPoolStub) Invalidate(_ context.Context, albumID uint64) error {
	p.invalidated = append(p.invalidated, albumID)
	delete(p.items, albumID)
	return nil
}

const (
	testPublicID = "Ab3dE6gH9j"
	testToken    = "0123456789abcdefghijABCD"
)

type randomLinkFixture struct {
	svc      *RandomLinkService
	links    *randomLinkStore
	pool     *randomPoolStub
	deps     RandomLinkDependencies
	rebuild  func(t *testing.T)
	albumErr error
}

func newRandomLinkServiceFixture(t *testing.T) *randomLinkFixture {
	t.Helper()
	f := &randomLinkFixture{links: newRandomLinkStore(), pool: &randomPoolStub{}}
	f.deps = RandomLinkDependencies{
		Links: f.links, Albums: randomAlbumStub{}, Users: publicIDStub{value: testPublicID},
		Storages: randomStorageStub{base: "https://cdn.example"}, Pool: f.pool,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	f.rebuild = func(t *testing.T) {
		t.Helper()
		svc, err := NewRandomLinkService(t.Context(), f.deps)
		if err != nil {
			t.Fatal(err)
		}
		f.svc = svc
	}
	f.rebuild(t)
	return f
}

// seedLink installs an enabled link on album 5 owned by enabled user 7.
func (f *randomLinkFixture) seedLink(candidates ...model.RandomCandidate) {
	publicID := testPublicID
	f.links.owners[7] = model.User{ID: 7, Status: model.UserStatusEnabled, PublicID: &publicID}
	f.links.links[5] = model.RandomLink{ID: 1, UserID: 7, AlbumID: 5, Token: testToken, Enabled: true}
	f.links.candidates = candidates
}

func candidate(path string) model.RandomCandidate {
	return model.RandomCandidate{StorageID: 3, Path: path, Ext: "jpg", HasWebP: true, HasOriginal: true}
}

func TestNewRandomLinkServiceRequiresDependencies(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	for name, strip := range map[string]func(*RandomLinkDependencies){
		"links": func(d *RandomLinkDependencies) { d.Links = nil }, "albums": func(d *RandomLinkDependencies) { d.Albums = nil },
		"users": func(d *RandomLinkDependencies) { d.Users = nil }, "storages": func(d *RandomLinkDependencies) { d.Storages = nil },
		"pool": func(d *RandomLinkDependencies) { d.Pool = nil }, "logger": func(d *RandomLinkDependencies) { d.Logger = nil },
	} {
		deps := f.deps
		strip(&deps)
		if _, err := NewRandomLinkService(t.Context(), deps); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("missing %s accepted: %v", name, err)
		}
	}
}

func TestGetWithoutLinkIsNil(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	view, err := f.svc.Get(t.Context(), 7, 5)
	if err != nil || view != nil {
		t.Fatalf("get = %+v err=%v, want nil view", view, err)
	}
}

func TestGetForeignAlbum(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.deps.Albums = randomAlbumStub{err: ErrForbidden}
	f.rebuild(t)
	if _, err := f.svc.Get(t.Context(), 7, 5); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign get = %v, want forbidden", err)
	}
	if _, err := f.svc.Put(t.Context(), 7, 5, true); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign put = %v, want forbidden", err)
	}
	if _, err := f.svc.Reset(t.Context(), 7, 5); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign reset = %v, want forbidden", err)
	}
	if err := f.svc.Delete(t.Context(), 7, 5); !errors.Is(err, ErrForbidden) {
		t.Fatalf("foreign delete = %v, want forbidden", err)
	}
	if f.links.touched {
		t.Fatal("link storage was reached for a foreign album")
	}
}

func TestPutCreates(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	view, err := f.svc.Put(t.Context(), 7, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	if !view.Enabled || view.CreatedAt.IsZero() {
		t.Fatalf("view = %+v", view)
	}
	if !regexp.MustCompile(`^/random/[0-9A-Za-z]{10}/[0-9A-Za-z]{24}$`).MatchString(view.Path) {
		t.Fatalf("path = %q", view.Path)
	}
	if !strings.HasPrefix(view.Path, "/random/"+testPublicID+"/") {
		t.Fatalf("path %q does not carry the owner's public id", view.Path)
	}
	stored := f.links.links[5]
	if stored.UserID != 7 || !strings.HasSuffix(view.Path, "/"+stored.Token) {
		t.Fatalf("stored = %+v path=%q", stored, view.Path)
	}
	got, err := f.svc.Get(t.Context(), 7, 5)
	if err != nil || got == nil || got.Path != view.Path {
		t.Fatalf("get after put = %+v err=%v", got, err)
	}
}

func TestPutUpdatesEnabledKeepsToken(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	first, err := f.svc.Put(t.Context(), 7, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Put(t.Context(), 7, 5, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Enabled || second.Path != first.Path || f.links.createCalls != 1 {
		t.Fatalf("second = %+v first=%+v creates=%d", second, first, f.links.createCalls)
	}
}

func TestPutNewLinkInvalidatesPool(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.pool.items = map[uint64][]model.RandomCandidate{5: {candidate("stale")}}
	if _, err := f.svc.Put(t.Context(), 7, 5, true); err != nil {
		t.Fatal(err)
	}
	if len(f.pool.invalidated) != 1 || f.pool.invalidated[0] != 5 {
		t.Fatalf("invalidated = %v, want [5]", f.pool.invalidated)
	}
}

func TestPutConcurrentCreateReturnsExisting(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	racing := &racingLinkStore{randomLinkStore: f.links}
	f.deps.Links = racing
	f.rebuild(t)
	view, err := f.svc.Put(t.Context(), 7, 5, true)
	if err != nil {
		t.Fatalf("lost create race = %v, want the winner's link", err)
	}
	if view.Path != "/random/"+testPublicID+"/"+testToken {
		t.Fatalf("path = %q", view.Path)
	}
}

// racingLinkStore simulates another request inserting the album's link
// between this request's lookup and its insert.
type racingLinkStore struct{ *randomLinkStore }

func (s *racingLinkStore) Create(_ context.Context, link model.RandomLink) (model.RandomLink, error) {
	s.links[link.AlbumID] = model.RandomLink{ID: 9, UserID: link.UserID, AlbumID: link.AlbumID, Token: testToken, Enabled: true}
	return model.RandomLink{}, model.ErrRandomLinkExists
}

func TestPutTokenCollisionRetries(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.links.createErrs = []error{model.ErrRandomLinkExists, model.ErrRandomLinkExists, nil}
	if _, err := f.svc.Put(t.Context(), 7, 5, true); err != nil {
		t.Fatalf("two token collisions = %v, want success on the third attempt", err)
	}
	if f.links.createCalls != 3 {
		t.Fatalf("create calls = %d, want 3", f.links.createCalls)
	}

	f = newRandomLinkServiceFixture(t)
	f.links.createErrs = []error{model.ErrRandomLinkExists, model.ErrRandomLinkExists, model.ErrRandomLinkExists, model.ErrRandomLinkExists, model.ErrRandomLinkExists, nil}
	if _, err := f.svc.Put(t.Context(), 7, 5, true); err == nil {
		t.Fatal("five straight collisions succeeded")
	}
	if f.links.createCalls != 5 {
		t.Fatalf("create calls = %d, want 5", f.links.createCalls)
	}
}

func TestResetChangesToken(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	if _, err := f.svc.Reset(t.Context(), 7, 5); !errors.Is(err, ErrNotFound) {
		t.Fatalf("reset without link = %v, want not found", err)
	}
	first, err := f.svc.Put(t.Context(), 7, 5, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := f.svc.Reset(t.Context(), 7, 5)
	if err != nil {
		t.Fatal(err)
	}
	if second.Path == first.Path || !regexp.MustCompile(`^/random/[0-9A-Za-z]{10}/[0-9A-Za-z]{24}$`).MatchString(second.Path) {
		t.Fatalf("reset path = %q (was %q)", second.Path, first.Path)
	}
}

func TestDeleteIdempotentAndInvalidates(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	if _, err := f.svc.Put(t.Context(), 7, 5, true); err != nil {
		t.Fatal(err)
	}
	f.pool.invalidated = nil
	for range 2 {
		if err := f.svc.Delete(t.Context(), 7, 5); err != nil {
			t.Fatal(err)
		}
	}
	if _, ok := f.links.links[5]; ok {
		t.Fatal("link survived delete")
	}
	if len(f.pool.invalidated) == 0 || f.pool.invalidated[0] != 5 {
		t.Fatalf("invalidated = %v", f.pool.invalidated)
	}
}

func TestPickUniformNotFound(t *testing.T) {
	other := "ZZZZZZZZZZ"
	cases := map[string]func(*randomLinkFixture) (uid, token string){
		"unknown token": func(f *randomLinkFixture) (string, string) {
			return testPublicID, "ZZZZZZZZZZZZZZZZZZZZZZZZ"
		},
		"uid of another account": func(f *randomLinkFixture) (string, string) { return other, testToken },
		"owner without public id": func(f *randomLinkFixture) (string, string) {
			f.links.owners[7] = model.User{ID: 7, Status: model.UserStatusEnabled}
			return testPublicID, testToken
		},
		"disabled link": func(f *randomLinkFixture) (string, string) {
			link := f.links.links[5]
			link.Enabled = false
			f.links.links[5] = link
			return testPublicID, testToken
		},
		"disabled owner": func(f *randomLinkFixture) (string, string) {
			owner := f.links.owners[7]
			owner.Status = model.UserStatusDisabled
			f.links.owners[7] = owner
			return testPublicID, testToken
		},
		"empty album": func(f *randomLinkFixture) (string, string) {
			f.links.candidates = nil
			return testPublicID, testToken
		},
		"malformed uid":   func(f *randomLinkFixture) (string, string) { return "short", testToken },
		"malformed token": func(f *randomLinkFixture) (string, string) { return testPublicID, "short" },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			f := newRandomLinkServiceFixture(t)
			f.seedLink(candidate("2026/10/a"))
			uid, token := arrange(f)
			if got, err := f.svc.Pick(t.Context(), uid, token, false); !errors.Is(err, ErrNotFound) || got != "" {
				t.Fatalf("pick = %q err=%v, want not found", got, err)
			}
		})
	}
}

func TestPickUsesPool(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.seedLink(candidate("2026/10/a"))
	if _, err := f.svc.Pick(t.Context(), testPublicID, testToken, false); err != nil {
		t.Fatal(err)
	}
	if len(f.links.candidateCalls) != 1 || f.links.candidateCalls[0] != (candidateCall{albumID: 5, limit: 5000}) {
		t.Fatalf("candidate calls = %v, want one call for album 5 with limit 5000", f.links.candidateCalls)
	}
	if f.pool.sets != 1 || f.pool.setTTL != 60*time.Second {
		t.Fatalf("pool sets=%d ttl=%v, want one 60s fill", f.pool.sets, f.pool.setTTL)
	}
	if _, err := f.svc.Pick(t.Context(), testPublicID, testToken, false); err != nil {
		t.Fatal(err)
	}
	if len(f.links.candidateCalls) != 1 {
		t.Fatalf("pool hit still queried candidates: %v", f.links.candidateCalls)
	}
}

func TestPickCachesEmptyAlbum(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.seedLink()
	for range 3 {
		if _, err := f.svc.Pick(t.Context(), testPublicID, testToken, false); !errors.Is(err, ErrNotFound) {
			t.Fatalf("empty album = %v", err)
		}
	}
	if len(f.links.candidateCalls) != 1 {
		t.Fatalf("empty album queried %d times, want 1", len(f.links.candidateCalls))
	}
}

func TestPickVersionFallback(t *testing.T) {
	for _, tc := range []struct {
		webp, original, wantOriginal bool
		want                         string
	}{
		{true, true, false, "https://cdn.example/2026/10/a.webp"},
		{true, true, true, "https://cdn.example/2026/10/a.jpg"},
		{false, true, false, "https://cdn.example/2026/10/a.jpg"},
		{true, false, true, "https://cdn.example/2026/10/a.webp"},
	} {
		f := newRandomLinkServiceFixture(t)
		f.seedLink(model.RandomCandidate{StorageID: 3, Path: "2026/10/a", Ext: "jpg", HasWebP: tc.webp, HasOriginal: tc.original})
		got, err := f.svc.Pick(t.Context(), testPublicID, testToken, tc.wantOriginal)
		if err != nil || got != tc.want {
			t.Fatalf("webp=%v original=%v ask-original=%v: got %q err=%v, want %q", tc.webp, tc.original, tc.wantOriginal, got, err, tc.want)
		}
	}
}

func TestPickEscapesPath(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.deps.Storages = randomStorageStub{base: "https://cdn.example/"}
	f.rebuild(t)
	f.seedLink(candidate("相册 1/a#b?c"))
	got, err := f.svc.Pick(t.Context(), testPublicID, testToken, false)
	if want := "https://cdn.example/%E7%9B%B8%E5%86%8C%201/a%23b%3Fc.webp"; err != nil || got != want {
		t.Fatalf("got %q err=%v, want %q", got, err, want)
	}
}

func TestPickSelectsByIntn(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	var asked int
	f.deps.Intn = func(n int) int { asked = n; return 2 }
	f.rebuild(t)
	f.seedLink(candidate("a"), candidate("b"), candidate("c"))
	got, err := f.svc.Pick(t.Context(), testPublicID, testToken, false)
	if err != nil || got != "https://cdn.example/c.webp" || asked != 3 {
		t.Fatalf("got %q err=%v asked=%d", got, err, asked)
	}
}

func TestPickDefaultRandomCoversCandidates(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.seedLink(candidate("a"), candidate("b"))
	seen := map[string]bool{}
	for range 200 {
		got, err := f.svc.Pick(t.Context(), testPublicID, testToken, false)
		if err != nil {
			t.Fatal(err)
		}
		seen[got] = true
	}
	if len(seen) != 2 {
		t.Fatalf("200 picks only produced %v", seen)
	}
}

func TestPickPoolErrorsDegrade(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.seedLink(candidate("a"))
	f.pool.getErr = errors.New("pool unavailable")
	f.pool.setErr = errors.New("pool unavailable")
	got, err := f.svc.Pick(t.Context(), testPublicID, testToken, false)
	if err != nil || got != "https://cdn.example/a.webp" {
		t.Fatalf("pool outage broke the redirect: %q err=%v", got, err)
	}
}

func TestPickStorageMissing(t *testing.T) {
	f := newRandomLinkServiceFixture(t)
	f.deps.Storages = randomStorageStub{err: model.ErrNotFound}
	f.rebuild(t)
	f.seedLink(candidate("a"))
	if _, err := f.svc.Pick(t.Context(), testPublicID, testToken, false); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing storage = %v, want not found", err)
	}
}

func TestValidRandomSegment(t *testing.T) {
	if !ValidRandomSegment("abcDEF0123", 10) {
		t.Fatal("valid segment rejected")
	}
	for _, bad := range []string{"", "abcDEF012", "abcDEF01234", "abcDEF-123", "abcDEF/123", "abcDEF%123", "abcDEF012é", "abcDEF01\x00"} {
		if ValidRandomSegment(bad, 10) {
			t.Fatalf("%q accepted", bad)
		}
	}
}
