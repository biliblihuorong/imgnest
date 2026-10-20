package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"

	httpapi "github.com/biliblihuorong/imgnest/internal/http"
	"github.com/biliblihuorong/imgnest/internal/service"
)

type panickingBatchImages struct{ *imagesStub }

func (s *panickingBatchImages) Get(ctx context.Context, subject service.TokenSubject, id uint64) (service.ImageView, error) {
	if id == 1 {
		panic("private-panic-payload")
	}
	return s.imagesStub.Get(ctx, subject, id)
}

func TestBatchRecoveryRedactsPanicAndContinuesOtherImages(t *testing.T) {
	for _, path := range []string{"/api/images/batch", "/api/trash/restore", "/api/trash/purge"} {
		t.Run(path, func(t *testing.T) {
			var logs bytes.Buffer
			router, err := httpapi.NewRouter(t.Context(), httpapi.Dependencies{
				Users: &usersStub{}, Tokens: &tokensStub{}, Images: &panickingBatchImages{&imagesStub{}},
				Logger: slog.New(slog.NewJSONHandler(&logs, nil)), Now: time.Now,
				Health: func(context.Context) error { return nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			body := `{"ids":[1,2]}`
			if strings.HasSuffix(path, "/batch") {
				body = `{"action":"delete","ids":[1,2]}`
			}
			response := request(t, router, "POST", path, body, testBearer)
			if response.Code != 207 {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			var result struct {
				Data []struct {
					ID      uint64 `json:"id"`
					Status  int    `json:"status"`
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"data"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(response.Body.String(), "private-panic-payload") || strings.Contains(logs.String(), "private-panic-payload") {
				t.Fatal("panic payload escaped the recovery boundary")
			}
			if len(result.Data) != 2 || result.Data[0].ID != 1 || result.Data[0].Status != 500 || result.Data[0].Code != 50001 || result.Data[0].Message != "internal error" || result.Data[1].ID != 2 || result.Data[1].Status != 200 || result.Data[1].Code != 0 {
				t.Fatalf("batch results=%+v", result.Data)
			}
		})
	}
}
