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
