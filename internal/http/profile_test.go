package http_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestPatchProfileUpdatesDisplayNameOnly(t *testing.T) {
	router, u, _, _ := routerFixture(t)
	resp := request(t, router, http.MethodPatch, "/api/auth/profile", `{"display_name":"  Nick  "}`, testBearer)
	expectCode(t, resp, 200, 0)
	if u.gotProfileID != 7 {
		t.Fatalf("profile user id=%d, want the authenticated identity 7", u.gotProfileID)
	}
	if u.gotProfileName != "  Nick  " {
		t.Fatalf("profile name=%q, want the raw request value", u.gotProfileName)
	}
	var data struct {
		DisplayName string `json:"display_name"`
	}
	if err := json.Unmarshal(envelope(t, resp)["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.DisplayName != "  Nick  " {
		t.Fatalf("response display name=%q", data.DisplayName)
	}
}

func TestPatchProfileRejectsUnknownFieldsAndAnonymous(t *testing.T) {
	router, u, _, _ := routerFixture(t)
	// The decoder rejects unknown fields, so role escalation has no request
	// shape at all.
	resp := request(t, router, http.MethodPatch, "/api/auth/profile",
		`{"display_name":"x","role":"admin","email":"evil@example.com"}`, testBearer)
	expectCode(t, resp, 400, 10001)
	if u.gotProfileID != 0 {
		t.Fatal("rejected request reached the service")
	}
	resp = request(t, router, http.MethodPatch, "/api/auth/profile", `{"display_name":"x"}`, "")
	expectCode(t, resp, 401, 20001)
}

func TestPatchProfileMapsServiceErrors(t *testing.T) {
	router, u, _, _ := routerFixture(t)
	u.err = service.ErrInvalidInput
	resp := request(t, router, http.MethodPatch, "/api/auth/profile", `{"display_name":"x"}`, testBearer)
	expectCode(t, resp, 400, 10001)
}

func TestPatchProfileRequiresPresentNonNullString(t *testing.T) {
	for _, body := range []string{`{}`, `null`, `{"display_name":null}`, `{"display_name":42}`, `{"display_name":false}`} {
		t.Run(body, func(t *testing.T) {
			router, users, _, _ := routerFixture(t)
			resp := request(t, router, http.MethodPatch, "/api/auth/profile", body, testBearer)
			expectCode(t, resp, 400, 10001)
			if users.gotProfileID != 0 {
				t.Fatal("malformed profile request reached service")
			}
		})
	}
	router, users, _, _ := routerFixture(t)
	resp := request(t, router, http.MethodPatch, "/api/auth/profile", `{"display_name":""}`, testBearer)
	expectCode(t, resp, 200, 0)
	if users.gotProfileID != 7 || users.gotProfileName != "" {
		t.Fatal("explicit empty display name must remain a valid clear")
	}
}
