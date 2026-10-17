package repo

import (
	"fmt"
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
	"gorm.io/gorm"
)

func TestTouchTokenRecordsUsage(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		user, users, tokens := grantFixture(t, db, "usage")
		api, err := tokens.CreateToken(t.Context(), testToken(user.ID), testGrant(user.PasswordHash))
		if err != nil {
			t.Fatal(err)
		}
		day := time.Date(2026, 10, 4, 23, 0, 0, 0, time.UTC)
		for _, use := range []model.TokenUse{
			{TokenID: api.ID, UserID: user.ID, Kind: model.TokenKindAPI, At: day, IP: "203.0.113.1", Requests: 3},
			{TokenID: api.ID, UserID: user.ID, Kind: model.TokenKindAPI, At: day.Add(time.Minute), IP: "203.0.113.2", Requests: 2},
			{TokenID: api.ID, UserID: user.ID, Kind: model.TokenKindAPI, At: day.Add(2 * time.Hour), IP: "203.0.113.2", Requests: 4},
		} {
			if err := tokens.TouchToken(t.Context(), use); err != nil {
				t.Fatal(err)
			}
		}
		found, err := tokens.FindToken(t.Context(), api.ID)
		if err != nil || found.UseCount != 9 || found.LastUsedIP != "203.0.113.2" || !found.LastUsedAt.Equal(day.Add(2*time.Hour)) {
			t.Fatalf("token usage not persisted: %+v %v", found, err)
		}
		usage, err := tokens.ListAPIUsage(t.Context(), user.ID, day.AddDate(0, 0, -1))
		if err != nil {
			t.Fatal(err)
		}
		want := []model.APIUsageDay{{UserID: user.ID, Day: "2026-10-04", Requests: 5}, {UserID: user.ID, Day: "2026-10-05", Requests: 4}}
		if fmt.Sprint(usage) != fmt.Sprint(want) {
			t.Fatalf("usage = %+v, want %+v", usage, want)
		}
		if later, err := tokens.ListAPIUsage(t.Context(), user.ID, day.AddDate(0, 0, 1)); err != nil || len(later) != 1 {
			t.Fatalf("since filter = %+v %v", later, err)
		}
		// API use is not web presence.
		if account, err := users.FindUserByID(t.Context(), user.ID); err != nil || account.LastSeenAt != nil {
			t.Fatalf("api use changed last_seen_at: %+v %v", account.LastSeenAt, err)
		}
		web := testToken(user.ID)
		web.Kind = model.TokenKindWeb
		web.TokenHash = "ffeeddccbb"
		session, err := tokens.CreateToken(t.Context(), web, testGrant(user.PasswordHash))
		if err != nil {
			t.Fatal(err)
		}
		seen := day.Add(3 * time.Hour)
		if err := tokens.TouchToken(t.Context(), model.TokenUse{TokenID: session.ID, UserID: user.ID, Kind: model.TokenKindWeb, At: seen, Requests: 1}); err != nil {
			t.Fatal(err)
		}
		account, err := users.FindUserByID(t.Context(), user.ID)
		if err != nil || account.LastSeenAt == nil || !account.LastSeenAt.Equal(seen) {
			t.Fatalf("web use did not refresh last_seen_at: %+v %v", account.LastSeenAt, err)
		}
		if usage, _ := tokens.ListAPIUsage(t.Context(), user.ID, day.AddDate(0, 0, -1)); len(usage) != 2 || usage[1].Requests != 4 {
			t.Fatalf("web use counted as api usage: %+v", usage)
		}
	})
}

func TestCreateTokenRecordsLoginHistory(t *testing.T) {
	forEachRepoDatabase(t, func(t *testing.T, db *gorm.DB) {
		user, users, tokens := grantFixture(t, db, "logins")
		start := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
		for i := range LoginHistoryLimit + 3 {
			token := testToken(user.ID)
			token.Kind = model.TokenKindWeb
			token.TokenHash = fmt.Sprintf("%064d", i)
			grant := testGrant(user.PasswordHash)
			grant.Login = &model.LoginEvent{IP: fmt.Sprintf("198.51.100.%d", i), UserAgent: "test", CreatedAt: start.Add(time.Duration(i) * time.Minute)}
			if _, err := tokens.CreateToken(t.Context(), token, grant); err != nil {
				t.Fatal(err)
			}
		}
		events, err := tokens.ListLoginEvents(t.Context(), user.ID, 100)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != LoginHistoryLimit {
			t.Fatalf("kept %d sign-ins, want %d", len(events), LoginHistoryLimit)
		}
		newest := start.Add(time.Duration(LoginHistoryLimit+2) * time.Minute)
		if events[0].IP != fmt.Sprintf("198.51.100.%d", LoginHistoryLimit+2) || !events[0].CreatedAt.Equal(newest) || events[0].UserAgent != "test" {
			t.Fatalf("newest sign-in = %+v", events[0])
		}
		account, err := users.FindUserByID(t.Context(), user.ID)
		if err != nil || account.LastSeenAt == nil || !account.LastSeenAt.Equal(newest) {
			t.Fatalf("sign-in did not refresh last_seen_at: %+v %v", account.LastSeenAt, err)
		}
		// A token created without a login (an API key) records nothing.
		if _, err := tokens.CreateToken(t.Context(), testToken(user.ID), testGrant(user.PasswordHash)); err != nil {
			t.Fatal(err)
		}
		if again, _ := tokens.ListLoginEvents(t.Context(), user.ID, 100); len(again) != LoginHistoryLimit || again[0].ID != events[0].ID {
			t.Fatalf("api key creation recorded a sign-in")
		}
	})
}
