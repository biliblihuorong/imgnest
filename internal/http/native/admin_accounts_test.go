package native_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/service"
)

type accountAdminStub struct {
	adminServiceStub
	created service.AdminUserInput
	caller  uint64
}

func (s *accountAdminStub) CreateUser(_ context.Context, caller uint64, input service.AdminUserInput) (service.UserView, error) {
	s.caller, s.created = caller, input
	return service.UserView{ID: 9, Username: input.Username, Email: input.Email, DisplayName: input.DisplayName,
		Role: input.Role, Status: input.Status, GroupID: input.GroupID}, s.err
}

func TestAdminCreateUserRouteContract(t *testing.T) {
	svc := &accountAdminStub{}
	router := adminRouter(t, svc, &adminImagesStub{})
	body := `{"username":"created","email":"created@example.com","password":"initial-password","display_name":"Display","role":"admin","status":"disabled","group_id":3}`
	for _, test := range []struct {
		bearer string
		status int
	}{{"", http.StatusUnauthorized}, {userBearer, http.StatusForbidden}, {adminBearer, http.StatusCreated}} {
		response := adminRequest(t, router, http.MethodPost, "/api/admin/users", body, test.bearer)
		if response.Code != test.status {
			t.Fatalf("create status=%d want=%d body=%s", response.Code, test.status, response.Body.String())
		}
		if test.status == http.StatusCreated {
			if svc.caller != 1 || svc.created.Password != "initial-password" || svc.created.DisplayName != "Display" || svc.created.Role != "admin" {
				t.Fatalf("create input=%+v caller=%d", svc.created, svc.caller)
			}
			var view service.UserView
			if err := json.Unmarshal(envelopeOf(t, response)["data"], &view); err != nil {
				t.Fatal(err)
			}
			if view.ID != 9 || view.Email != "created@example.com" {
				t.Fatalf("view=%+v", view)
			}
			if strings.Contains(response.Body.String(), "password") {
				t.Fatal("create response leaked credentials")
			}
		}
	}
}

func TestAdminAccountBodiesRejectUnknownAndNullFields(t *testing.T) {
	for _, method := range []string{http.MethodPost, http.MethodPatch} {
		path := "/api/admin/users"
		if method == http.MethodPatch {
			path += "/8"
		}
		for _, body := range []string{`null`, `{"user_id":42}`, `{"auth_version":0}`, `{"used_bytes":0}`, `{"password_hash":"hash"}`, `{"registered_ip":"192.0.2.1"}`, `{"avatar_provider":"gravatar"}`, `{"role":null}`, `{"display_name":null}`, `{"username":null}`} {
			t.Run(method+body, func(t *testing.T) {
				router := adminRouter(t, &accountAdminStub{}, &adminImagesStub{})
				response := adminRequest(t, router, method, path, body, adminBearer)
				if response.Code != http.StatusBadRequest {
					t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
				}
			})
		}
	}
	router := adminRouter(t, &accountAdminStub{}, &adminImagesStub{})
	response := adminRequest(t, router, http.MethodPatch, "/api/admin/users/8", `{"password":"replacement-password"}`, adminBearer)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("password edit accepted: %s", response.Body.String())
	}
}
