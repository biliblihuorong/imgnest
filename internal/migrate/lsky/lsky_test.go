package lsky

import (
	"context"
	"crypto/md5" // #nosec G501 -- mirrors Lsky's content checksum in fixtures.
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/config"
	"github.com/biliblihuorong/imgnest/internal/migrate"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"gorm.io/gorm"
)

// bcryptY is a well-formed 60-byte Lsky ($2y$) bcrypt digest.
const bcryptY = "$2y$10$abcdefghijklmnopqrstuuJ8tvgAGvqqh1Rz8rLKd8e0Lk0cPa3XW"

func target(t *testing.T) (*gorm.DB, uint64) {
	t.Helper()
	ctx := t.Context()
	db, err := repo.Open(ctx, config.Database{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "target.db"), MaxOpen: 1, MaxIdle: 1, BusyTimeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := migrate.Up(ctx, sqlDB, "sqlite"); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO storages (name, driver, config, base_url) VALUES ('local', 'local', '{}', 'http://localhost/i/1')`).Error; err != nil {
		t.Fatal(err)
	}
	var policy model.Policy
	if err := db.Exec(`INSERT INTO policies (storage_id, name) VALUES (1, 'blog')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Order("id DESC").First(&policy).Error; err != nil {
		t.Fatal(err)
	}
	return db, policy.ID
}

func snapshot() Snapshot {
	created := time.Date(2024, 4, 12, 14, 44, 55, 0, time.UTC)
	return Snapshot{
		Groups: []Group{
			{ID: 1, Name: "默认组", IsDefault: true, MaxFileKB: 5120, PerMinute: 20, AcceptedSuffix: []string{"jpeg", "jpg", "png", "ico", "psd", "tif", "webp"}},
			{ID: 3, Name: "博客组", MaxFileKB: 51200, PerMinute: 9999},
		},
		Users: []User{
			{ID: 1, GroupID: 3, Name: "某科学的贝壳", Email: "Shell@Example.com", PasswordHash: bcryptY, IsAdmin: true, Enabled: true, RegisteredIP: "192.0.2.1", CreatedAt: created},
			{ID: 3, GroupID: 3, Name: "涩涩", Email: "short@example.com", PasswordHash: bcryptY, Enabled: true},
			{ID: 5, GroupID: 9, Name: "test", Email: "test@example.com", PasswordHash: "plain", Enabled: false},
		},
		Tokens: []Token{
			{ID: 1, UserID: 1, Name: "picgo", Hash: strings.Repeat("ab", 32), CreatedAt: created},
			{ID: 2, UserID: 99, Name: "orphan", Hash: strings.Repeat("cd", 32)},
		},
		Albums: []Album{{ID: 4, UserID: 1, Name: "博客", Intro: "封面"}},
	}
}

func TestImportAccountsMapsAndIsIdempotent(t *testing.T) {
	db, policyID := target(t)
	ctx := t.Context()
	snap := snapshot()
	mapping, report, err := ImportAccounts(ctx, db, snap, Options{PolicyID: policyID})
	if err != nil {
		t.Fatal(err)
	}
	if report.GroupsCreated != 2 || report.UsersCreated != 3 || report.TokensCreated != 1 || report.AlbumsCreated != 1 {
		t.Fatalf("report = %+v", report)
	}
	var shell model.User
	if err := db.First(&shell, "id = ?", mapping.Users[1]).Error; err != nil {
		t.Fatal(err)
	}
	if shell.Username != "某科学的贝壳" || shell.Email != "shell@example.com" || shell.PasswordHash != bcryptY || shell.Role != model.UserRoleAdmin || shell.GroupID != mapping.Groups[3] || shell.RegisteredIP != "192.0.2.1" || !shell.CreatedAt.Equal(time.Date(2024, 4, 12, 14, 44, 55, 0, time.UTC)) {
		t.Fatalf("shell = %+v", shell)
	}
	var short model.User
	if err := db.First(&short, "id = ?", mapping.Users[3]).Error; err != nil {
		t.Fatal(err)
	}
	if short.Username != "short" || short.DisplayName != "涩涩" {
		t.Fatalf("two-rune name must fall back to the email local part: %+v", short)
	}
	var disabled model.User
	if err := db.First(&disabled, "id = ?", mapping.Users[5]).Error; err != nil {
		t.Fatal(err)
	}
	if disabled.PasswordHash != unusablePassword || disabled.Status != model.UserStatusDisabled || disabled.GroupID == mapping.Groups[3] {
		t.Fatalf("non-bcrypt, disabled user in a missing group = %+v", disabled)
	}
	var group model.Group
	if err := db.First(&group, "id = ?", mapping.Groups[1]).Error; err != nil {
		t.Fatal(err)
	}
	if group.IsDefault || group.MaxFileBytes != 5120*1024 || group.UploadPerMin != 20 || group.DefaultPolicyID != policyID || strings.Join(group.AllowedExts, ",") != "jpg,png,tiff,webp" {
		t.Fatalf("group = %+v", group)
	}
	var bound int64
	if err := db.Table("group_policies").Where("policy_id = ?", policyID).Count(&bound).Error; err != nil || bound != 2 {
		t.Fatalf("bound groups = %d, %v", bound, err)
	}
	var token model.Token
	if err := db.First(&token, "id = ?", 1).Error; err != nil {
		t.Fatal("token id must be kept so id|secret keeps working")
	}
	if token.UserID != mapping.Users[1] || token.TokenHash != strings.Repeat("ab", 32) || token.Kind != model.TokenKindAPI {
		t.Fatalf("token = %+v", token)
	}
	var album model.Album
	if err := db.First(&album, "id = ?", mapping.Albums[4]).Error; err != nil || album.UserID != mapping.Users[1] || album.NameSearch == nil {
		t.Fatalf("album = %+v, %v", album, err)
	}

	again, second, err := ImportAccounts(ctx, db, snap, Options{PolicyID: policyID})
	if err != nil {
		t.Fatal(err)
	}
	if second.GroupsCreated+second.UsersCreated+second.TokensCreated+second.AlbumsCreated != 0 || second.UsersReused != 3 || second.TokensKept != 1 {
		t.Fatalf("rerun report = %+v", second)
	}
	for id, target := range mapping.Users {
		if again.Users[id] != target {
			t.Fatal("rerun must map onto the same accounts")
		}
	}
}

func TestImportAccountsRenumbersTakenTokenID(t *testing.T) {
	db, policyID := target(t)
	ctx := t.Context()
	if err := db.Exec(`INSERT INTO users (id, username, email, password_hash, role, group_id, status) VALUES (7, 'existing', 'existing@example.com', 'x', 'user', 1, 'enabled')`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO tokens (id, user_id, name, token_hash, kind, abilities) VALUES (1, 7, 'web', 'ff', 'web', '["*"]')`).Error; err != nil {
		t.Fatal(err)
	}
	_, report, err := ImportAccounts(ctx, db, snapshot(), Options{PolicyID: policyID})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(report.Notes, "\n"), "Token 1 的新编号") {
		t.Fatalf("renumbered token must be reported: %v", report.Notes)
	}
}

func TestImportAccountsRefusesWebPOnlyRule(t *testing.T) {
	db, policyID := target(t)
	if err := db.Exec(`UPDATE policies SET webp_mode = 'webp_only' WHERE id = ?`, policyID).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := ImportAccounts(t.Context(), db, snapshot(), Options{PolicyID: policyID}); err == nil {
		t.Fatal("a WebP-only rule drops originals and breaks historic links")
	}
}

type mapFetcher map[string][]byte

func (m mapFetcher) Fetch(_ context.Context, image Image, _ Strategy) ([]byte, error) {
	data, ok := m[image.Pathname]
	if !ok {
		return nil, errors.New("missing")
	}
	return data, nil
}

func sum(data []byte) string {
	digest := md5.Sum(data) // #nosec G401 -- fixture checksum.
	return hex.EncodeToString(digest[:])
}

func TestImportImagesFiltersVerifiesAndSkipsImported(t *testing.T) {
	db, _ := target(t)
	ctx := t.Context()
	good, other := []byte("good-bytes"), []byte("other-bytes")
	snap := Snapshot{
		Users: []User{{ID: 1, Enabled: true}, {ID: 2, Enabled: false}},
		Images: []Image{
			{ID: 1, UserID: 1, AlbumID: 4, Pathname: "1/2025/07/25/a.webp", MD5: sum(good), IsPublic: true},
			{ID: 2, UserID: 1, Pathname: "1/2025/07/25/b.ico"},
			{ID: 3, UserID: 1, Pathname: "1/2025/07/25/c.jpeg"},
			{ID: 4, UserID: 1, Pathname: "1/2025/07/25/d.PNG"},
			{ID: 5, UserID: 2, Pathname: "2/x.png"},
			{ID: 6, UserID: 0, Pathname: "guest.png"},
			{ID: 7, UserID: 1, Pathname: "1/2025/07/25/e.png", MD5: sum(good)},
			{ID: 8, UserID: 1, Pathname: "1/2025/07/25/f.png", MD5: sum(other)},
		},
	}
	fetch := mapFetcher{"1/2025/07/25/a.webp": good, "1/2025/07/25/e.png": other, "1/2025/07/25/f.png": other}
	mapping := Mapping{Users: map[int64]uint64{1: 10, 2: 11}, Albums: map[int64]uint64{4: 40}}
	var got []Item
	ingest := func(_ context.Context, item Item) error {
		got = append(got, item)
		if item.Path == "1/2025/07/25/f" {
			return errors.New("pipeline refused")
		}
		return nil
	}
	report, err := ImportImages(ctx, db, snap, mapping, fetch, ingest, ImageOptions{StorageID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.Imported != 1 || len(report.Skipped) != 5 || len(report.Failed) != 2 {
		t.Fatalf("report = %+v", report)
	}
	if len(got) != 2 || got[0].Path != "1/2025/07/25/a" || got[0].Ext != "webp" || got[0].UserID != 10 || got[0].AlbumID != 40 || string(got[0].Data) != "good-bytes" {
		t.Fatalf("items = %+v", got)
	}
	if !strings.Contains(report.Failed[0], "MD5") || !strings.Contains(report.Failed[1], "pipeline refused") {
		t.Fatalf("failures = %v", report.Failed)
	}
	if err := db.Exec(`INSERT INTO images (key, user_id, policy_id, storage_id, path, ext, mime, state, operation_id) VALUES ('k', 0, 1, 1, '1/2025/07/25/a', 'webp', 'image/webp', 'active', 'op')`).Error; err != nil {
		t.Fatal(err)
	}
	got = nil
	report, err = ImportImages(ctx, db, snap, mapping, fetch, ingest, ImageOptions{StorageID: 1, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if report.AlreadyThere != 1 || len(report.Failed) != 1 || !strings.Contains(report.Failed[0], "e.png") || len(got) != 0 {
		t.Fatalf("rerun must skip the imported path and honour the limit: %+v %+v", report, got)
	}
}

func TestHTTPFetcherEscapesPathAndRejectsErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.EscapedPath() {
		case "/ShellBlogImg/1/%E5%8D%9A%E5%AE%A2/a%20b.webp":
			_, _ = w.Write([]byte("payload"))
		case "/ShellBlogImg/big.webp":
			_, _ = w.Write([]byte(strings.Repeat("x", 2<<20)))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	fetch := HTTPFetcher{Client: server.Client(), MaxBytes: 1 << 20}
	strategy := Strategy{ID: 9, URL: server.URL + "/ShellBlogImg"}
	data, err := fetch.Fetch(t.Context(), Image{Pathname: "1/博客/a b.webp"}, strategy)
	if err != nil || string(data) != "payload" {
		t.Fatalf("fetch = %q, %v", data, err)
	}
	if _, err := fetch.Fetch(t.Context(), Image{Pathname: "missing.webp"}, strategy); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("missing object = %v", err)
	}
	if _, err := fetch.Fetch(t.Context(), Image{Pathname: "big.webp"}, strategy); err == nil {
		t.Fatal("oversized download must fail")
	}
	if _, err := (HTTPFetcher{}).Fetch(t.Context(), Image{Pathname: "a.webp"}, Strategy{URL: "file:///etc"}); err == nil {
		t.Fatal("non-HTTP strategy URL must be refused")
	}
}

func TestDirFetcherStaysInsideRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "1"), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "1", "a.webp"), []byte("ok"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetch := DirFetcher{Root: root}
	if data, err := fetch.Fetch(t.Context(), Image{Pathname: "1/a.webp"}, Strategy{}); err != nil || string(data) != "ok" {
		t.Fatalf("fetch = %q, %v", data, err)
	}
	if _, err := fetch.Fetch(t.Context(), Image{Pathname: "../outside.webp"}, Strategy{}); err == nil {
		t.Fatal("a pathname must not escape the source directory")
	}
}

func TestHelpers(t *testing.T) {
	if got := uniqidTime("63d342d7133e6.png"); !got.Equal(time.Unix(0x63d342d7, 0).UTC()) {
		t.Fatalf("uniqid time = %v", got)
	}
	for _, name := range []string{"short.png", "zzzzzzzz133e6.png", "00000001133e6.png"} {
		if !uniqidTime(name).IsZero() {
			t.Fatalf("%s must not yield a time", name)
		}
	}
	if dir, ext := splitPathname("1/2025/07/25/a.webp"); dir != "1/2025/07/25/a" || ext != "webp" {
		t.Fatalf("split = %q %q", dir, ext)
	}
	if dir, ext := splitPathname("noext"); dir != "" || ext != "" {
		t.Fatal("a pathname without extension has no object key")
	}
	if !isBcrypt(bcryptY) || isBcrypt("$10$"+bcryptY[7:]) || isBcrypt("plain") {
		t.Fatal("bcrypt detection")
	}
}
