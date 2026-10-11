package lsky_test

import (
	"crypto/md5"  // #nosec G501 -- verifies the public legacy content checksum contract.
	"crypto/sha1" // #nosec G505 -- public legacy content checksum contract.
	"encoding/hex"
	"encoding/json"
	"io"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
)

// v1Data decodes the data member of a v1 envelope.
func v1Data(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var envelope struct {
		Status  bool           `json:"status"`
		Message string         `json:"message"`
		Data    map[string]any `json:"data"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("invalid v1 envelope: %v", err)
	}
	return envelope.Data
}

func TestV1TokensContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			// Fixture A: exactly three issuance attempts, then the revoke flow.
			fixture := newV1Fixture(t, driver)
			form := "application/x-www-form-urlencoded"

			issued := fixture.request(t, "POST", "/api/v1/tokens", "email=alice@example.com&password="+alicePassword, form, "")
			mustStatus(t, issued, 200)
			assertGolden(t, "tokens_success.json", issued.Body.Bytes())

			// JSON bodies are the exact alternative of the form encoding.
			jsonBody := `{"email":"alice@example.com","password":"` + alicePassword + `"}`
			issuedJSON := fixture.request(t, "POST", "/api/v1/tokens", jsonBody, "application/json", "")
			mustStatus(t, issuedJSON, 200)
			assertGolden(t, "tokens_success.json", issuedJSON.Body.Bytes())

			token := fixture.login(t)
			revoked := fixture.request(t, "DELETE", "/api/v1/tokens", "", "", token)
			mustStatus(t, revoked, 200)
			assertGolden(t, "tokens_revoked.json", revoked.Body.Bytes())
			after := fixture.request(t, "GET", "/api/v1/profile", "", "", token)
			mustStatus(t, after, 401)
			assertGolden(t, "error_401.json", after.Body.Bytes())

			// Fixture B: three failing attempts stay business failures.
			other := newV1Fixture(t, driver)
			wrong := other.request(t, "POST", "/api/v1/tokens", "email=alice@example.com&password=not-the-password", form, "")
			mustStatus(t, wrong, 200)
			assertGolden(t, "tokens_bad_credentials.json", wrong.Body.Bytes())

			unknown := other.request(t, "POST", "/api/v1/tokens", "email=nobody@example.com&password=whatever-password", form, "")
			mustStatus(t, unknown, 200)
			assertGolden(t, "tokens_bad_credentials.json", unknown.Body.Bytes())

			missing := other.request(t, "POST", "/api/v1/tokens", "email=alice@example.com", form, "")
			mustStatus(t, missing, 200)
			assertGolden(t, "tokens_bad_credentials.json", missing.Body.Bytes())
		})
	}
}

func TestV1TokensThrottle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newV1Fixture(t, driver)
			form := "application/x-www-form-urlencoded"
			body := "email=alice@example.com&password=not-the-password"
			// Three attempts per IP per minute; the fourth is rejected.
			for range 3 {
				response := fixture.request(t, "POST", "/api/v1/tokens", body, form, "")
				mustStatus(t, response, 200)
				assertGolden(t, "tokens_bad_credentials.json", response.Body.Bytes())
			}
			fourth := fixture.request(t, "POST", "/api/v1/tokens", body, form, "")
			mustStatus(t, fourth, 429)
			assertGolden(t, "tokens_throttled.json", fourth.Body.Bytes())
		})
	}
}

func TestV1StrategiesContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newV1Fixture(t, driver)

			// Guest uploads are disabled by migration default: anonymous
			// callers see an empty strategy list.
			closed := fixture.request(t, "GET", "/api/v1/strategies", "", "", "")
			mustStatus(t, closed, 200)
			assertGolden(t, "strategies_empty.json", closed.Body.Bytes())

			// An invalid bearer token must never downgrade to guest.
			bad := fixture.request(t, "GET", "/api/v1/strategies", "", "", "9|totallyinvalidtoken0123456789abcdefg")
			mustStatus(t, bad, 401)
			assertGolden(t, "error_401.json", bad.Body.Bytes())

			loggedIn := fixture.request(t, "GET", "/api/v1/strategies", "", "", fixture.login(t))
			mustStatus(t, loggedIn, 200)
			assertGolden(t, "strategies_list.json", loggedIn.Body.Bytes())

			fixture.enableGuestUpload(t)
			guest := fixture.request(t, "GET", "/api/v1/strategies", "", "", "")
			mustStatus(t, guest, 200)
			assertGolden(t, "strategies_list.json", guest.Body.Bytes())
		})
	}
}

func TestV1UploadContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newV1Fixture(t, driver)
			source := pngBytes(t, 32, 16)
			token := fixture.login(t)

			// Guest uploads are closed: anonymous upload is unauthenticated.
			closed := fixture.upload(t, "travel.png", source, "")
			mustStatus(t, closed, 401)
			assertGolden(t, "error_401.json", closed.Body.Bytes())

			// A broken token must never silently fall back to guest mode.
			invalid := fixture.upload(t, "travel.png", source, "9|totallyinvalidtoken0123456789abcdefg")
			mustStatus(t, invalid, 401)
			assertGolden(t, "error_401.json", invalid.Body.Bytes())

			missing := fixture.upload(t, "", source, token)
			mustStatus(t, missing, 200)
			assertGolden(t, "upload_missing_file.json", missing.Body.Bytes())

			uploaded := fixture.upload(t, "travel.png", source, token, "permission", "1")
			mustStatus(t, uploaded, 200)
			assertGolden(t, "upload_success.json", uploaded.Body.Bytes())
			assertUploadValues(t, fixture, uploaded.Body.Bytes(), "travel.png", true)

			// A strategy the caller's group does not hold is refused.
			stranger := fixture.upload(t, "travel.png", source, token, "strategy_id", "9999")
			mustStatus(t, stranger, 200)
			assertGolden(t, "upload_strategy_unavailable.json", stranger.Body.Bytes())

			// Format detection trusts the file header, not the extension:
			// real JPEG bytes are refused because the group allows png only.
			rejected := fixture.upload(t, "photo.png", jpegBytes(t, 16, 16), token)
			mustStatus(t, rejected, 200)
			assertGolden(t, "upload_format_rejected.json", rejected.Body.Bytes())

			// Quota is enforced against the group capacity in bytes.
			if err := fixture.db.Exec("UPDATE groups SET capacity_bytes = 1 WHERE id = ?", fixture.alice.GroupID).Error; err != nil {
				t.Fatal(err)
			}
			overQuota := fixture.upload(t, "travel.png", source, token)
			mustStatus(t, overQuota, 200)
			assertGolden(t, "upload_quota_failure.json", overQuota.Body.Bytes())
			if err := fixture.db.Exec("UPDATE groups SET capacity_bytes = ? WHERE id = ?", groupCapacity, fixture.alice.GroupID).Error; err != nil {
				t.Fatal(err)
			}

			// Guests share the very same reservation/compensation pipeline
			// with user_id = 0 once the administrator enables them.
			fixture.enableGuestUpload(t)
			guest := fixture.upload(t, "travel.png", source, "")
			mustStatus(t, guest, 200)
			assertGolden(t, "upload_success.json", guest.Body.Bytes())
			assertUploadValues(t, fixture, guest.Body.Bytes(), "travel.png", false)

			// Guests cannot file uploads into albums.
			guestAlbum := fixture.upload(t, "travel.png", source, "", "album_id", "1")
			mustStatus(t, guestAlbum, 200)
			assertGolden(t, "upload_guest_album.json", guestAlbum.Body.Bytes())
		})
	}
}

// assertUploadValues pins the field *values* the golden patterns cannot
// carry: the KB unit, the hashes of the exact stored bytes, pathname/name
// derivation, link consistency and ownership of the fresh upload.
func assertUploadValues(t *testing.T, fixture *v1Fixture, body []byte, originName string, owned bool) {
	t.Helper()
	data := v1Data(t, body)
	key, _ := data["key"].(string)
	if key == "" {
		t.Fatal("upload response lost the key")
	}
	var stored modelImage
	if err := fixture.db.Where("key = ?", key).First(&stored).Error; err != nil {
		t.Fatalf("upload row missing: %v", err)
	}
	if owned && stored.UserID != fixture.alice.ID {
		t.Fatalf("upload owner = %d, want alice %d", stored.UserID, fixture.alice.ID)
	}
	if !owned && stored.UserID != 0 {
		t.Fatalf("guest upload owner = %d, want the shared guest account 0", stored.UserID)
	}
	storedOriginal := readStoredObject(t, fixture, stored.Path+"."+stored.Ext)
	md5Sum := md5.Sum(storedOriginal)  // #nosec G401 -- public legacy content checksum contract.
	shaSum := sha1.Sum(storedOriginal) // #nosec G401 -- public legacy content checksum contract.
	if data["md5"] != hex.EncodeToString(md5Sum[:]) || data["sha1"] != hex.EncodeToString(shaSum[:]) {
		t.Fatalf("md5/sha1 do not describe the stored original: %v/%v vs %s/%s",
			data["md5"], data["sha1"], hex.EncodeToString(md5Sum[:]), hex.EncodeToString(shaSum[:]))
	}
	size, ok := data["size"].(float64)
	if !ok || size != float64(stored.Size)/1024 {
		t.Fatalf("size = %v, want the stored bytes in KB (%d/1024)", data["size"], stored.Size)
	}
	if data["pathname"] != stored.Path+"."+stored.Ext {
		t.Fatalf("pathname = %v, want %s.%s", data["pathname"], stored.Path, stored.Ext)
	}
	if data["name"] != path.Base(stored.Path)+"."+stored.Ext {
		t.Fatalf("name = %v, want the stored filename", data["name"])
	}
	if data["origin_name"] != originName {
		t.Fatalf("origin_name = %v, want %q", data["origin_name"], originName)
	}
	links, _ := data["links"].(map[string]any)
	if links == nil {
		t.Fatal("upload response lost links")
	}
	url, _ := links["url"].(string)
	if url == "" || links["url"] != links["webp_url"] {
		t.Fatalf("links.url must follow link_prefer=webp and equal webp_url: %v vs %v", links["url"], links["webp_url"])
	}
	wantHTML := "&lt;img src=\"" + url + "\" alt=\"" + originName + "\" title=\"" + originName + "\" /&gt;"
	if links["html"] != wantHTML {
		t.Fatalf("links.html = %q, want the Lsky escape format %q", links["html"], wantHTML)
	}
	if strings.Contains(links["html"].(string), "&#34;") || strings.Contains(links["html"].(string), "&#39;") {
		t.Fatal("links.html escaped the quotes; Lsky keeps them literal")
	}
	if links["thumbnail_url"] != strings.TrimSuffix(url, ".webp")+"_thumbs.webp" {
		t.Fatalf("thumbnail_url = %v, want the cloud _thumbs.webp", links["thumbnail_url"])
	}
	if stored.IsPublic != owned {
		t.Fatalf("is_public = %v, want %v", stored.IsPublic, owned)
	}
}

// readStoredObject reads the exact stored object bytes through the real
// driver (the on-disk file carries a transport envelope around the content).
func readStoredObject(t *testing.T, fixture *v1Fixture, key string) []byte {
	t.Helper()
	object, _, err := fixture.local.Open(t.Context(), key)
	if err != nil {
		t.Fatalf("open stored object %s: %v", key, err)
	}
	defer func() {
		if err := object.Close(); err != nil {
			t.Error(err)
		}
	}()
	data, err := io.ReadAll(object)
	if err != nil {
		t.Fatalf("read stored object %s: %v", key, err)
	}
	return data
}

func TestV1ImagesContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := seedImages(t, driver)
			token := fixture.login(t)

			page := fixture.request(t, "GET", "/api/v1/images", "", "", token)
			mustStatus(t, page, 200)
			assertGolden(t, "images_page.json", page.Body.Bytes())

			// The Lsky quirk: album_id absent or 0 both mean unassigned only.
			zero := fixture.request(t, "GET", "/api/v1/images?album_id=0", "", "", token)
			mustStatus(t, zero, 200)
			assertGolden(t, "images_page.json", zero.Body.Bytes())

			// Newest first: beta was uploaded after gamma.
			if names := v1OriginNames(t, page.Body.Bytes()); len(names) != 2 || names[0] != "beta.png" || names[1] != "gamma.png" {
				t.Fatalf("newest order = %v, want [beta.png gamma.png]", names)
			}

			utmost := fixture.request(t, "GET", "/api/v1/images?order=utmost", "", "", token)
			mustStatus(t, utmost, 200)
			if names := v1OriginNames(t, utmost.Body.Bytes()); len(names) != 2 || names[0] != "gamma.png" || names[1] != "beta.png" {
				t.Fatalf("utmost order = %v, want the larger gamma.png first", names)
			}

			keyword := fixture.request(t, "GET", "/api/v1/images?keyword=beta", "", "", token)
			mustStatus(t, keyword, 200)
			assertGolden(t, "images_keyword_page.json", keyword.Body.Bytes())

			private := fixture.request(t, "GET", "/api/v1/images?permission=private", "", "", token)
			mustStatus(t, private, 200)
			assertGolden(t, "images_keyword_page.json", private.Body.Bytes())

			public := fixture.request(t, "GET", "/api/v1/images?permission=public", "", "", token)
			mustStatus(t, public, 200)
			if names := v1OriginNames(t, public.Body.Bytes()); len(names) != 1 || names[0] != "gamma.png" {
				t.Fatalf("public filter = %v, want only the public gamma.png", names)
			}

			albumPage := fixture.request(t, "GET", "/api/v1/images?album_id="+strconv.FormatUint(fixture.albumID, 10), "", "", token)
			mustStatus(t, albumPage, 200)
			assertGolden(t, "images_album_page.json", albumPage.Body.Bytes())

			empty := fixture.request(t, "GET", "/api/v1/images?page=2", "", "", token)
			mustStatus(t, empty, 200)
			assertGolden(t, "images_empty_page.json", empty.Body.Bytes())

			unauthenticated := fixture.request(t, "GET", "/api/v1/images", "", "", "")
			mustStatus(t, unauthenticated, 401)
			assertGolden(t, "error_401.json", unauthenticated.Body.Bytes())
		})
	}
}

func TestV1AlbumsContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := seedImages(t, driver)
			token := fixture.login(t)

			list := fixture.request(t, "GET", "/api/v1/albums", "", "", token)
			mustStatus(t, list, 200)
			assertGolden(t, "albums_page.json", list.Body.Bytes())

			most := fixture.request(t, "GET", "/api/v1/albums?order=most", "", "", token)
			mustStatus(t, most, 200)
			if names := v1AlbumNames(t, most.Body.Bytes()); len(names) != 2 || names[0] != "博客" || names[1] != "笔记" {
				t.Fatalf("most order = %v, want the album holding an image first", names)
			}

			keyword := fixture.request(t, "GET", "/api/v1/albums?keyword=%E5%8D%9A%E5%AE%A2", "", "", token)
			mustStatus(t, keyword, 200)
			if names := v1AlbumNames(t, keyword.Body.Bytes()); len(names) != 1 || names[0] != "博客" {
				t.Fatalf("keyword filter = %v, want only 博客", names)
			}

			// Deleting an album keeps its images and clears their reference.
			deleted := fixture.request(t, "DELETE", "/api/v1/albums/"+strconv.FormatUint(fixture.albumID, 10), "", "", token)
			mustStatus(t, deleted, 200)
			assertGolden(t, "album_deleted.json", deleted.Body.Bytes())

			var remaining, stillAssigned int64
			if err := fixture.db.Model(&albumRow{}).Where("id = ?", fixture.albumID).Count(&remaining).Error; err != nil || remaining != 0 {
				t.Fatalf("album row survived deletion (count=%d err=%v)", remaining, err)
			}
			if err := fixture.db.Model(&imageRow{}).Where("album_id = ?", fixture.albumID).Count(&stillAssigned).Error; err != nil || stillAssigned != 0 {
				t.Fatalf("album deletion left assigned images (count=%d err=%v)", stillAssigned, err)
			}
			gone := fixture.request(t, "GET", "/api/v1/images?album_id="+strconv.FormatUint(fixture.albumID, 10), "", "", token)
			mustStatus(t, gone, 200)
			assertGolden(t, "images_album_gone.json", gone.Body.Bytes())

			missing := fixture.request(t, "DELETE", "/api/v1/albums/"+strconv.FormatUint(fixture.albumID, 10), "", "", token)
			mustStatus(t, missing, 200)
			assertGolden(t, "albums_missing.json", missing.Body.Bytes())

			foreign := fixture.request(t, "DELETE", "/api/v1/albums/"+strconv.FormatUint(fixture.foreignAlbumID, 10), "", "", token)
			mustStatus(t, foreign, 200)
			assertGolden(t, "albums_missing.json", foreign.Body.Bytes())

			unauthenticated := fixture.request(t, "DELETE", "/api/v1/albums/1", "", "", "")
			mustStatus(t, unauthenticated, 401)
			assertGolden(t, "error_401.json", unauthenticated.Body.Bytes())
		})
	}
}

func TestV1ProfileContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := seedImages(t, driver)
			token := fixture.login(t)

			profile := fixture.request(t, "GET", "/api/v1/profile", "", "", token)
			mustStatus(t, profile, 200)
			assertGolden(t, "profile.json", profile.Body.Bytes())

			// used_capacity is the charged account usage in KB.
			var used int64
			if err := fixture.db.Table("users").Select("used_bytes").Where("id = ?", fixture.alice.ID).Scan(&used).Error; err != nil {
				t.Fatal(err)
			}
			if used <= 0 {
				t.Fatal("fixture uploads did not charge capacity")
			}
			data := v1Data(t, profile.Body.Bytes())
			if value, ok := data["used_capacity"].(float64); !ok || value != float64(used)/1024 {
				t.Fatalf("used_capacity = %v, want %d/1024", data["used_capacity"], used)
			}

			unauthenticated := fixture.request(t, "GET", "/api/v1/profile", "", "", "")
			mustStatus(t, unauthenticated, 401)
			assertGolden(t, "error_401.json", unauthenticated.Body.Bytes())
		})
	}
}

func TestV1APIDisabledRejectsEveryRoute(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := seedImages(t, driver)
			token := fixture.login(t)
			if err := fixture.db.Exec("UPDATE settings SET value = 'false' WHERE key = 'api_enabled'").Error; err != nil {
				t.Fatal(err)
			}
			cases := []struct{ method, target, body, contentType string }{
				{"POST", "/api/v1/tokens", "email=alice@example.com&password=" + alicePassword, "application/x-www-form-urlencoded"},
				{"DELETE", "/api/v1/tokens", "", ""},
				{"GET", "/api/v1/strategies", "", ""},
				{"POST", "/api/v1/upload", "", "multipart/form-data; boundary=v1boundary"},
				{"GET", "/api/v1/images", "", ""},
				{"DELETE", "/api/v1/images/GDmF2b", "", ""},
				{"GET", "/api/v1/albums", "", ""},
				{"DELETE", "/api/v1/albums/1", "", ""},
				{"GET", "/api/v1/profile", "", ""},
			}
			for _, testCase := range cases {
				response := fixture.request(t, testCase.method, testCase.target, testCase.body, testCase.contentType, token)
				mustStatus(t, response, 403)
				assertGolden(t, "error_403.json", response.Body.Bytes())
			}
		})
	}
}

// v1OriginNames reads the origin_name sequence of an images page.
func v1OriginNames(t *testing.T, body []byte) []string {
	t.Helper()
	data := v1Data(t, body)
	items, _ := data["data"].([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]any)
		name, _ := entry["origin_name"].(string)
		names = append(names, name)
	}
	return names
}

// v1AlbumNames reads the name sequence of an albums page.
func v1AlbumNames(t *testing.T, body []byte) []string {
	t.Helper()
	data := v1Data(t, body)
	items, _ := data["data"].([]any)
	names := make([]string, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]any)
		name, _ := entry["name"].(string)
		names = append(names, name)
	}
	return names
}

func TestV1ImageDeleteContract(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			fixture := newV1Fixture(t, driver)
			token := fixture.login(t)

			uploaded := fixture.upload(t, "gone.png", pngBytes(t, 24, 24), token)
			mustStatus(t, uploaded, 200)
			key, _ := v1Data(t, uploaded.Body.Bytes())["key"].(string)
			if key == "" {
				t.Fatal("upload response missing image key")
			}

			// Success moves the image into the recycle bin with an empty object.
			deleted := fixture.request(t, "DELETE", "/api/v1/images/"+key, "", "", token)
			mustStatus(t, deleted, 200)
			assertGolden(t, "image_deleted.json", deleted.Body.Bytes())

			// Re-deleting the trashed key stays idempotent (HTTP 200 success).
			repeat := fixture.request(t, "DELETE", "/api/v1/images/"+key, "", "", token)
			mustStatus(t, repeat, 200)
			assertGolden(t, "image_deleted.json", repeat.Body.Bytes())

			// A key that never existed stays a business failure (HTTP 200).
			missing := fixture.request(t, "DELETE", "/api/v1/images/00000000000000000000000000000000", "", "", token)
			mustStatus(t, missing, 200)
			assertGolden(t, "image_missing.json", missing.Body.Bytes())

			anonymous := fixture.request(t, "DELETE", "/api/v1/images/"+key, "", "", "")
			mustStatus(t, anonymous, 401)
			assertGolden(t, "error_401.json", anonymous.Body.Bytes())

			// Another owner's key answers exactly like a missing one, so the
			// endpoint leaks neither key existence nor ownership.
			bob, err := fixture.users.Register(t.Context(), service.RegisterInput{Username: "bob", Email: "bob@example.com", Password: "bob-v1-delete-password"}) // #nosec G101 -- throwaway fixture account, not a credential.
			if err != nil {
				t.Fatal(err)
			}
			bobLogin := fixture.request(t, "POST", "/api/v1/tokens", "email=bob@example.com&password=bob-v1-delete-password", "application/x-www-form-urlencoded", "")
			var bobBody struct {
				Data struct {
					Token string `json:"token"`
				} `json:"data"`
			}
			if err := json.Unmarshal(bobLogin.Body.Bytes(), &bobBody); err != nil || bobBody.Data.Token == "" {
				t.Fatalf("bob login failed: %s", bobLogin.Body.String())
			}
			bobUpload := fixture.upload(t, "kept.png", pngBytes(t, 24, 24), bobBody.Data.Token)
			mustStatus(t, bobUpload, 200)
			bobKey, _ := v1Data(t, bobUpload.Body.Bytes())["key"].(string)
			if bobKey == "" {
				t.Fatal("bob upload response missing image key")
			}
			foreign := fixture.request(t, "DELETE", "/api/v1/images/"+bobKey, "", "", token)
			mustStatus(t, foreign, 200)
			if foreign.Body.String() != missing.Body.String() {
				t.Fatalf("foreign key delete differs from missing key:\n%s\n%s", foreign.Body.String(), missing.Body.String())
			}

			// Even an administrator's v1 token stays owner-scoped on delete.
			if err := fixture.db.Exec("UPDATE users SET role = ? WHERE id = ?", model.UserRoleAdmin, fixture.alice.ID).Error; err != nil {
				t.Fatal(err)
			}
			adminForeign := fixture.request(t, "DELETE", "/api/v1/images/"+bobKey, "", "", token)
			mustStatus(t, adminForeign, 200)
			if adminForeign.Body.String() != missing.Body.String() {
				t.Fatalf("admin foreign key delete differs from missing key:\n%s", adminForeign.Body.String())
			}
			var kept modelImage
			if err := fixture.db.First(&kept, "key = ?", bobKey).Error; err != nil || kept.State != model.ImageStateActive || kept.UserID != bob.ID {
				t.Fatalf("cross-owner delete trashed the image: state=%s owner=%d err=%v", kept.State, kept.UserID, err)
			}
		})
	}
}
