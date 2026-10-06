package http_test

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/repo"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// These unrelated capabilities deliberately have no implementation: account
// handlers must never call storage or image operations.
type accountOnlyDependencies struct {
	service.StorageProvider
	service.SecretCodec
	service.TemplateValidator
	native.AdminImages
}

func TestManagedAccountHTTPDatabaseLifecycle(t *testing.T) {
	for _, driver := range []string{"sqlite", "postgres"} {
		t.Run(driver, func(t *testing.T) {
			db := imageDatabase(t, driver)
			ctx := t.Context()
			accounts, err := repo.NewAdminRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			userRepo, err := repo.NewUserRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			settings, err := repo.NewSettingsRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			tokenRepo, err := repo.NewTokenRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			storageRepo, err := repo.NewStorageRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			policyRepo, err := repo.NewPolicyRepository(ctx, db)
			if err != nil {
				t.Fatal(err)
			}
			users, err := service.NewUserService(ctx, userRepo, settings)
			if err != nil {
				t.Fatal(err)
			}
			tokens, err := service.NewTokenService(ctx, tokenRepo, userRepo, settings, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			unused := accountOnlyDependencies{}
			admin, err := service.NewAdminService(ctx, service.AdminDependencies{Users: accounts, Groups: accounts, References: accounts,
				Storages: storageRepo, Policies: policyRepo, Settings: settings, Provision: &service.ProvisionService{}, Drivers: unused, Secrets: unused, Templates: unused, Now: time.Now})
			if err != nil {
				t.Fatal(err)
			}
			owner, err := users.InitAdmin(ctx, service.RegisterInput{Username: "owner", Email: "owner@example.com", Password: "owner-password-123"})
			if err != nil {
				t.Fatal(err)
			}
			issue := func(email, password string) string {
				t.Helper()
				proof, err := users.VerifyCredentials(ctx, email, password)
				if err != nil {
					t.Fatal(err)
				}
				token, err := tokens.Issue(ctx, proof.Subject, service.TokenInput{Name: "test", Kind: "web", Abilities: []string{"*"}})
				if err != nil {
					t.Fatal(err)
				}
				return token.Token
			}
			ownerToken := issue(owner.Email, "owner-password-123")
			handler, err := native.NewHandler(ctx, users, tokens, time.Now)
			if err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			if err := handler.RegisterRoutes(ctx, router); err != nil {
				t.Fatal(err)
			}
			if err := handler.RegisterAdminRoutes(ctx, router, admin, unused); err != nil {
				t.Fatal(err)
			}
			registration, err := settings.RegistrationEnabled(ctx)
			if err != nil || registration {
				t.Fatalf("fixture registration=%t %v", registration, err)
			}
			createBody := `{"username":"  managed  ","email":" MANAGED@EXAMPLE.COM ","password":"initial-password-123","display_name":" Nick ","role":"user","status":"enabled","group_id":` + strconv.FormatUint(owner.GroupID, 10) + `}`
			response := request(t, router, http.MethodPost, "/api/admin/users", createBody, ownerToken)
			expectCode(t, response, 201, 0)
			var created service.UserView
			if err := json.Unmarshal(envelope(t, response)["data"], &created); err != nil {
				t.Fatal(err)
			}
			if created.Username != "managed" || created.Email != "managed@example.com" || created.DisplayName != "Nick" || created.AvatarURL == nil {
				t.Fatalf("created=%+v", created)
			}
			if strings.Contains(response.Body.String(), "password") || strings.Contains(response.Body.String(), "registered_ip") || strings.Contains(response.Body.String(), "auth_version") {
				t.Fatal("private account data leaked")
			}
			managedToken := issue(created.Email, "initial-password-123")
			path := "/api/admin/users/" + strconv.FormatUint(created.ID, 10)
			expectCode(t, request(t, router, http.MethodPost, "/api/admin/users", createBody, ownerToken), 409, 30002)
			expectCode(t, request(t, router, http.MethodPatch, path, `{"role":"admin"}`, managedToken), 403, 20003)
			for _, body := range []string{`{}`, `{"display_name":null}`} {
				expectCode(t, request(t, router, http.MethodPatch, "/api/auth/profile", body, managedToken), 400, 10001)
			}
			expectCode(t, request(t, router, http.MethodPatch, path, `{"username":"owner","status":"disabled"}`, ownerToken), 409, 30002)
			profile := request(t, router, http.MethodGet, "/api/auth/me", "", managedToken)
			expectCode(t, profile, 200, 0)
			var current service.UserView
			if err := json.Unmarshal(envelope(t, profile)["data"], &current); err != nil {
				t.Fatal(err)
			}
			if current.DisplayName != "Nick" || current.Status != "enabled" {
				t.Fatalf("malformed/failed patch changed account: %+v", current)
			}
			expectCode(t, request(t, router, http.MethodPatch, path, `{"display_name":"New Nick"}`, ownerToken), 200, 0)
			expectCode(t, request(t, router, http.MethodGet, "/api/auth/me", "", managedToken), 200, 0)
			changed := request(t, router, http.MethodPatch, path, `{"email":"new@example.com","role":"admin"}`, ownerToken)
			expectCode(t, changed, 200, 0)
			var updated service.UserView
			if err := json.Unmarshal(envelope(t, changed)["data"], &updated); err != nil {
				t.Fatal(err)
			}
			if updated.AvatarURL == nil || *updated.AvatarURL == *created.AvatarURL {
				t.Fatal("email change did not refresh derived avatar")
			}
			expectCode(t, request(t, router, http.MethodGet, "/api/auth/me", "", managedToken), 401, 20001)
			expectCode(t, request(t, router, http.MethodPost, "/api/auth/login", `{"email":"managed@example.com","password":"initial-password-123"}`, ""), 401, 20002)
			newToken := issue("new@example.com", "initial-password-123")
			ownerPath := "/api/admin/users/" + strconv.FormatUint(owner.ID, 10)
			expectCode(t, request(t, router, http.MethodPatch, ownerPath, `{"status":"disabled"}`, ownerToken), 403, 20003)
			expectCode(t, request(t, router, http.MethodPatch, ownerPath, `{"role":"user"}`, ownerToken), 200, 0)
			expectCode(t, request(t, router, http.MethodGet, "/api/auth/me", "", ownerToken), 401, 20001)
			expectCode(t, request(t, router, http.MethodPatch, path, `{"role":"user"}`, newToken), 403, 20003)
		})
	}
}
