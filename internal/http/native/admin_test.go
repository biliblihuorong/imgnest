package native_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	adminBearer = "admin-session-token"
	userBearer  = "ordinary-session-token"
)

type adminTokensStub struct{ native.TokenService }

func (adminTokensStub) Authenticate(_ context.Context, raw string) (service.Identity, error) {
	switch raw {
	case adminBearer:
		return service.Identity{User: service.UserView{ID: 1, Role: model.UserRoleAdmin, Status: model.UserStatusEnabled}, TokenID: 1, Kind: "web"}, nil
	case userBearer:
		return service.Identity{User: service.UserView{ID: 2, Role: model.UserRoleUser, Status: model.UserStatusEnabled}, TokenID: 2, Kind: "web"}, nil
	}
	return service.Identity{}, service.ErrUnauthenticated
}

type adminServiceStub struct {
	native.AdminService
	err        error
	keyword    string
	page, size int
	userID     uint64
	patch      service.AdminUserPatch
	purged     int64
	purgeErr   error
	sample     string
	previewErr error
}

func (s *adminServiceStub) ListUsers(_ context.Context, page, size int, keyword string) (service.AdminUserPage, error) {
	s.page, s.size, s.keyword = page, size, keyword
	if s.err != nil {
		return service.AdminUserPage{}, s.err
	}
	return service.AdminUserPage{Items: []service.UserView{{ID: 8, Username: "bob", Email: "bob@example.com", Role: "user", Status: "enabled", GroupID: 1}}, Total: 1, Page: page, Size: size}, nil
}
func (s *adminServiceStub) PatchUser(_ context.Context, _, _ uint64, patch service.AdminUserPatch) (service.UserView, error) {
	s.patch = patch
	if s.err != nil {
		return service.UserView{}, s.err
	}
	return service.UserView{ID: 8, Username: "bob", Status: "disabled", GroupID: 3}, nil
}
func (s *adminServiceStub) DeleteGroup(context.Context, uint64) error { return s.err }
func (s *adminServiceStub) ListGroups(context.Context) ([]service.GroupView, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []service.GroupView{}, nil
}
func (s *adminServiceStub) CreateGroup(context.Context, service.GroupInput) (service.GroupView, error) {
	if s.err != nil {
		return service.GroupView{}, s.err
	}
	return service.GroupView{}, nil
}
func (s *adminServiceStub) PatchGroup(context.Context, uint64, service.GroupPatch) (service.GroupView, error) {
	if s.err != nil {
		return service.GroupView{}, s.err
	}
	return service.GroupView{}, nil
}
func (s *adminServiceStub) CreateStorage(context.Context, service.StorageInput) (service.StorageView, error) {
	if s.err != nil {
		return service.StorageView{}, s.err
	}
	return service.StorageView{}, nil
}
func (s *adminServiceStub) PatchStorage(context.Context, uint64, service.StoragePatch) (service.StorageView, error) {
	if s.err != nil {
		return service.StorageView{}, s.err
	}
	return service.StorageView{}, nil
}
func (s *adminServiceStub) TestStorage(context.Context, uint64) (service.StorageTestResult, error) {
	if s.err != nil {
		return service.StorageTestResult{}, s.err
	}
	return service.StorageTestResult{OK: true, Checks: service.StorageChecks{Put: true, Copy: true, Delete: true}}, nil
}
func (s *adminServiceStub) CreatePolicy(context.Context, service.PolicyPatch) (model.Policy, error) {
	if s.err != nil {
		return model.Policy{}, s.err
	}
	return model.Policy{}, nil
}
func (s *adminServiceStub) PatchPolicy(context.Context, uint64, service.PolicyPatch) (model.Policy, error) {
	if s.err != nil {
		return model.Policy{}, s.err
	}
	return model.Policy{}, nil
}
func (s *adminServiceStub) DeletePolicy(context.Context, uint64) error  { return s.err }
func (s *adminServiceStub) DeleteStorage(context.Context, uint64) error { return s.err }
func (s *adminServiceStub) ListStorages(context.Context) ([]service.StorageView, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []service.StorageView{{ID: 5, Name: "cloud", Driver: "s3", BaseURL: "https://cdn.test", Enabled: true}}, nil
}
func (s *adminServiceStub) ListPolicies(context.Context) ([]model.Policy, error) {
	if s.err != nil {
		return nil, s.err
	}
	return []model.Policy{}, nil
}
func (s *adminServiceStub) PreviewPolicy(_ context.Context, _ uint64, _, _ string) (string, error) {
	if s.previewErr != nil {
		return "", s.previewErr
	}
	return s.sample, nil
}
func (s *adminServiceStub) GetSettings(context.Context) (service.AdminSettingsView, error) {
	if s.err != nil {
		return service.AdminSettingsView{}, s.err
	}
	return service.AdminSettingsView{SiteName: "ImgNest", TrashDays: 7, APIEnabled: true}, nil
}
func (s *adminServiceStub) PutSettings(context.Context, service.SettingsPatch) (service.AdminSettingsView, error) {
	if s.err != nil {
		return service.AdminSettingsView{}, s.err
	}
	return service.AdminSettingsView{SiteName: "ImgNest", TrashDays: 7, APIEnabled: true}, nil
}

type adminImagesStub struct {
	native.AdminImages
	purged   int64
	purgeErr error
	list     service.ImagePage
	listErr  error
}

func (s *adminImagesStub) AdminList(context.Context, service.TokenSubject, service.AdminImageQuery) (service.ImagePage, error) {
	return s.list, s.listErr
}
func (s *adminImagesStub) AdminTrash(context.Context, service.TokenSubject, uint64) error {
	return nil
}
func (s *adminImagesStub) AdminPurgeAll(context.Context, service.TokenSubject) (int64, error) {
	return s.purged, s.purgeErr
}

func adminRouter(t *testing.T, svc native.AdminService, images native.AdminImages) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, adminTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterAdminRoutes(t.Context(), router, svc, images); err != nil {
		t.Fatal(err)
	}
	return router
}

func adminRequest(t *testing.T, router http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func adminRoutes() []struct {
	method, path, body string
	created            bool
} {
	return []struct {
		method, path, body string
		created            bool
	}{
		{"GET", "/api/admin/users", "", false},
		{"PATCH", "/api/admin/users/8", `{}`, false},
		{"GET", "/api/admin/groups", "", false},
		{"POST", "/api/admin/groups", `{}`, true},
		{"PATCH", "/api/admin/groups/3", `{}`, false},
		{"DELETE", "/api/admin/groups/3", "", false},
		{"GET", "/api/admin/storages", "", false},
		{"POST", "/api/admin/storages", `{}`, true},
		{"PATCH", "/api/admin/storages/5", `{}`, false},
		{"DELETE", "/api/admin/storages/5", "", false},
		{"POST", "/api/admin/storages/5/test", "", false},
		{"GET", "/api/admin/policies", "", false},
		{"POST", "/api/admin/policies", `{}`, true},
		{"POST", "/api/admin/policies/preview", `{"path_tpl":"{Y}","name_tpl":"{uniqid}"}`, false},
		{"PATCH", "/api/admin/policies/9", `{}`, false},
		{"DELETE", "/api/admin/policies/9", "", false},
		{"GET", "/api/admin/settings", "", false},
		{"PUT", "/api/admin/settings", `{}`, false},
		{"GET", "/api/admin/images", "", false},
		{"DELETE", "/api/admin/images/12", "", false},
		{"POST", "/api/admin/trash/purge-all", "", false},
	}
}

func TestAdminRoutesGuardAndStatusCodes(t *testing.T) {
	for _, route := range adminRoutes() {
		t.Run(route.method+" "+route.path, func(t *testing.T) {
			router := adminRouter(t, &adminServiceStub{}, &adminImagesStub{})

			anonymous := adminRequest(t, router, route.method, route.path, route.body, "")
			if anonymous.Code != http.StatusUnauthorized {
				t.Fatalf("anonymous status=%d", anonymous.Code)
			}
			expectAdminCode(t, anonymous, 20001)

			ordinary := adminRequest(t, router, route.method, route.path, route.body, userBearer)
			if ordinary.Code != http.StatusForbidden {
				t.Fatalf("non-admin status=%d", ordinary.Code)
			}
			expectAdminCode(t, ordinary, 20003)

			admin := adminRequest(t, router, route.method, route.path, route.body, adminBearer)
			want := http.StatusOK
			if route.created {
				want = http.StatusCreated
			}
			if admin.Code != want {
				t.Fatalf("admin status=%d want %d body=%s", admin.Code, want, admin.Body.String())
			}
			expectAdminCode(t, admin, 0)
		})
	}
}

func expectAdminCode(t *testing.T, w *httptest.ResponseRecorder, code int) {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid envelope %s: %v", w.Body.String(), err)
	}
	for _, key := range []string{"code", "message", "data"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("missing %s in %s", key, w.Body.String())
		}
	}
	var got int
	if err := json.Unmarshal(body["code"], &got); err != nil || got != code {
		t.Fatalf("code=%d want %d body=%s", got, code, w.Body.String())
	}
}

func TestAdminReferenceConflictsMapToNewCodes(t *testing.T) {
	router := adminRouter(t, &adminServiceStub{err: service.ErrGroupHasMembers}, &adminImagesStub{})
	w := adminRequest(t, router, "DELETE", "/api/admin/groups/3", "", adminBearer)
	if w.Code != http.StatusConflict {
		t.Fatalf("group members status=%d", w.Code)
	}
	expectAdminCode(t, w, 30008)

	router = adminRouter(t, &adminServiceStub{err: service.ErrStillReferenced}, &adminImagesStub{})
	w = adminRequest(t, router, "DELETE", "/api/admin/storages/5", "", adminBearer)
	if w.Code != http.StatusConflict {
		t.Fatalf("storage referenced status=%d", w.Code)
	}
	expectAdminCode(t, w, 30009)
}

func TestAdminStorageListingNeverEchoesConfig(t *testing.T) {
	router := adminRouter(t, &adminServiceStub{}, &adminImagesStub{})
	w := adminRequest(t, router, "GET", "/api/admin/storages", "", adminBearer)
	expectAdminCode(t, w, 0)
	var data []map[string]json.RawMessage
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &data); err != nil {
		t.Fatal(err)
	}
	if len(data) != 1 {
		t.Fatalf("storage items=%s", w.Body.String())
	}
	fields := data[0]
	for _, key := range []string{"id", "name", "driver", "base_url", "enabled"} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("storage view missing %s: %s", key, w.Body.String())
		}
	}
	for key := range fields {
		if strings.Contains(key, "config") || strings.Contains(key, "secret") {
			t.Fatalf("storage view leaked %s: %s", key, w.Body.String())
		}
	}
	if len(fields) != 5 {
		t.Fatalf("storage view exposes %d fields: %s", len(fields), w.Body.String())
	}
}

func envelopeOf(t *testing.T, w *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var body map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func TestAdminUserListPaginationContract(t *testing.T) {
	svc := &adminServiceStub{}
	router := adminRouter(t, svc, &adminImagesStub{})
	w := adminRequest(t, router, "GET", "/api/admin/users?page=2&size=5&keyword=bob", "", adminBearer)
	expectAdminCode(t, w, 0)
	if svc.page != 2 || svc.size != 5 || svc.keyword != "bob" {
		t.Fatalf("query passthrough page=%d size=%d keyword=%q", svc.page, svc.size, svc.keyword)
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &data); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"items", "total", "page", "size"} {
		if _, ok := data[key]; !ok {
			t.Fatalf("user page missing %s: %s", key, w.Body.String())
		}
	}
	if !strings.Contains(string(data["items"]), `"used_bytes"`) || !strings.Contains(string(data["items"]), `"group_id"`) {
		t.Fatalf("admin user view fields=%s", data["items"])
	}
	if strings.Contains(w.Body.String(), "registered_ip") || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("admin user list leaked private fields: %s", w.Body.String())
	}

	bad := adminRequest(t, router, "GET", "/api/admin/users?page=0", "", adminBearer)
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid page status=%d", bad.Code)
	}
	expectAdminCode(t, bad, 10001)
}

func TestAdminPatchUserBodyShape(t *testing.T) {
	svc := &adminServiceStub{}
	router := adminRouter(t, svc, &adminImagesStub{})
	w := adminRequest(t, router, "PATCH", "/api/admin/users/8", `{"status":"disabled","group_id":3}`, adminBearer)
	expectAdminCode(t, w, 0)
	if svc.patch.Status == nil || *svc.patch.Status != "disabled" || svc.patch.GroupID == nil || *svc.patch.GroupID != 3 {
		t.Fatalf("patch passthrough=%+v", svc.patch)
	}
	var view service.UserView
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &view); err != nil {
		t.Fatal(err)
	}
	if view.ID != 8 || view.Status != "disabled" || view.GroupID != 3 {
		t.Fatalf("patched view=%+v", view)
	}
	rejected := adminRequest(t, router, "PATCH", "/api/admin/users/8", `{"role":"admin"}`, adminBearer)
	if rejected.Code != http.StatusBadRequest {
		t.Fatalf("role escalation accepted: %d", rejected.Code)
	}
	expectAdminCode(t, rejected, 10001)
}

func TestAdminPurgeAllPartialFailureSemantics(t *testing.T) {
	router := adminRouter(t, &adminServiceStub{}, &adminImagesStub{purged: 5})
	w := adminRequest(t, router, "POST", "/api/admin/trash/purge-all", "", adminBearer)
	if w.Code != http.StatusOK {
		t.Fatalf("purge-all status=%d", w.Code)
	}
	expectAdminCode(t, w, 0)
	var data struct {
		Purged int64 `json:"purged"`
	}
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &data); err != nil || data.Purged != 5 {
		t.Fatalf("purged payload=%s err=%v", w.Body.String(), err)
	}

	router = adminRouter(t, &adminServiceStub{}, &adminImagesStub{purged: 3, purgeErr: service.ErrStorage})
	w = adminRequest(t, router, "POST", "/api/admin/trash/purge-all", "", adminBearer)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("partial purge status=%d", w.Code)
	}
	expectAdminCode(t, w, 50002)
	data = struct {
		Purged int64 `json:"purged"`
	}{}
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &data); err != nil || data.Purged != 3 {
		t.Fatalf("partial purge payload=%s err=%v", w.Body.String(), err)
	}
}

func TestAdminPolicyPreviewContract(t *testing.T) {
	router := adminRouter(t, &adminServiceStub{sample: "2026/10/04/example"}, &adminImagesStub{})
	w := adminRequest(t, router, "POST", "/api/admin/policies/preview", `{"path_tpl":"{Y}","name_tpl":"{filename}"}`, adminBearer)
	expectAdminCode(t, w, 0)
	var data struct {
		Sample string `json:"sample"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Sample != "2026/10/04/example" || data.Error != "" {
		t.Fatalf("preview payload=%+v", data)
	}

	router = adminRouter(t, &adminServiceStub{previewErr: errors.New("unmatched template brace")}, &adminImagesStub{})
	w = adminRequest(t, router, "POST", "/api/admin/policies/preview", `{"path_tpl":"{broken}","name_tpl":"{uniqid}"}`, adminBearer)
	if w.Code != http.StatusOK {
		t.Fatalf("preview failure status=%d", w.Code)
	}
	expectAdminCode(t, w, 0)
	data = struct {
		Sample string `json:"sample"`
		Error  string `json:"error"`
	}{}
	if err := json.Unmarshal(envelopeOf(t, w)["data"], &data); err != nil {
		t.Fatal(err)
	}
	if data.Sample != "" || data.Error == "" {
		t.Fatalf("failed preview payload=%+v", data)
	}
}
