package native_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/http/native"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

type policyListStub struct {
	native.ImageService
	rules []service.PolicySummary
	err   error
}

func (s *policyListStub) ListPolicies(context.Context, service.TokenSubject) ([]service.PolicySummary, error) {
	return s.rules, s.err
}

type acceptingTokensStub struct{ native.TokenService }

func (acceptingTokensStub) Authenticate(_ context.Context, raw string) (service.Identity, error) {
	if raw != testCredential {
		return service.Identity{}, service.ErrUnauthenticated
	}
	return service.Identity{TokenID: 7, Kind: "web"}, nil
}

const testCredential = "7|accepted"

func policiesRouter(t *testing.T, images *policyListStub) *gin.Engine {
	t.Helper()
	handler, err := native.NewHandler(t.Context(), &siteUsersStub{}, acceptingTokensStub{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err := handler.RegisterImageRoutes(t.Context(), router, images, native.ImageOptions{}); err != nil {
		t.Fatal(err)
	}
	return router
}

func requestPolicies(t *testing.T, router http.Handler, bearer string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, "/api/policies", nil)
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	return w
}

func TestPolicyListRequiresAuthentication(t *testing.T) {
	router := policiesRouter(t, &policyListStub{})
	for _, tc := range []struct{ name, bearer string }{{"missing", ""}, {"invalid", "nope"}} {
		t.Run(tc.name, func(t *testing.T) {
			response := requestPolicies(t, router, tc.bearer)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d", response.Code)
			}
			var body struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != 20001 {
				t.Fatalf("code=%d", body.Code)
			}
		})
	}
}

func TestPolicyListPassesThroughPermittedRules(t *testing.T) {
	rules := []service.PolicySummary{{ID: 1, Name: "first"}, {ID: 2, Name: "wide"}}
	router := policiesRouter(t, &policyListStub{rules: rules})
	response := requestPolicies(t, router, testCredential)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	var envelope struct {
		Code int             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != 0 {
		t.Fatalf("code=%d", envelope.Code)
	}
	var decoded []map[string]json.RawMessage
	if err := json.Unmarshal(envelope.Data, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded) != len(rules) {
		t.Fatalf("rules %s", envelope.Data)
	}
	for index, rule := range decoded {
		if len(rule) != 2 {
			t.Fatalf("rule %d exposes %d fields: %s", index, len(rule), envelope.Data)
		}
		if _, ok := rule["id"]; !ok {
			t.Fatalf("rule %d lacks id: %s", index, envelope.Data)
		}
		if _, ok := rule["name"]; !ok {
			t.Fatalf("rule %d lacks name: %s", index, envelope.Data)
		}
	}
}

func TestPolicyListEmptyIsArray(t *testing.T) {
	router := policiesRouter(t, &policyListStub{rules: []service.PolicySummary{}})
	response := requestPolicies(t, router, testCredential)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d", response.Code)
	}
	body := response.Body.String()
	if !strings.Contains(body, `"data":[]`) || strings.Contains(body, `"data":null`) {
		t.Fatalf("empty policy list is not []: %s", body)
	}
}

func TestPolicyListErrorMapping(t *testing.T) {
	router := policiesRouter(t, &policyListStub{err: service.ErrForbidden})
	response := requestPolicies(t, router, testCredential)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status=%d", response.Code)
	}
	var body struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != 20003 {
		t.Fatalf("code=%d", body.Code)
	}
}
