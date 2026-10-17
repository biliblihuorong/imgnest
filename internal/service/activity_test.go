package service_test

import (
	"testing"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
)

func TestActivityRecordsSignInsAndAPIUsage(t *testing.T) {
	forDatabases(t, func(t *testing.T, fixture *authFixture) {
		user := registerUser(t, fixture, "activity")
		now := time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC)
		tokens := newTokenService(t, fixture, &now)
		activity, err := service.NewActivityService(t.Context(), fixture.users, fixture.tokens, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}
		ctx := service.WithClient(t.Context(), service.ClientInfo{IP: " 198.51.100.7 ", UserAgent: "Mozilla/5.0"})
		session, err := tokens.Issue(ctx, passwordSubject(t, fixture, user), service.TokenInput{Name: "web", Kind: service.TokenKindWeb, Abilities: []string{"*"}})
		if err != nil {
			t.Fatal(err)
		}
		key, err := tokens.Issue(t.Context(), passwordSubject(t, fixture, user), tokenInput())
		if err != nil {
			t.Fatal(err)
		}
		apiCtx := service.WithClient(t.Context(), service.ClientInfo{IP: "203.0.113.9"})
		// The first use is written at once; the next two are buffered until
		// a use a minute later writes all of them.
		for _, offset := range []time.Duration{0, time.Second, 2 * time.Second, time.Minute + 3*time.Second} {
			now = time.Date(2026, 10, 17, 9, 30, 0, 0, time.UTC).Add(offset)
			if _, err := tokens.Authenticate(apiCtx, key.Token); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := tokens.Authenticate(ctx, session.Token); err != nil {
			t.Fatal(err)
		}
		view, err := activity.Activity(t.Context(), user.ID)
		if err != nil {
			t.Fatal(err)
		}
		if view.LastLogin == nil || view.LastLogin.IP != "198.51.100.7" || view.LastLogin.UserAgent != "Mozilla/5.0" {
			t.Fatalf("last login = %+v", view.LastLogin)
		}
		if view.LastSeenAt == nil || !view.LastSeenAt.Equal(now) {
			t.Fatalf("last seen = %v, want %v", view.LastSeenAt, now)
		}
		if view.ActiveSessions != 1 || len(view.Tokens) != 1 {
			t.Fatalf("sessions=%d tokens=%+v", view.ActiveSessions, view.Tokens)
		}
		if got := view.Tokens[0]; got.UseCount != 4 || got.LastUsedIP != "203.0.113.9" {
			t.Fatalf("api token usage = %+v", got)
		}
		if len(view.APIUsage) != service.ActivityDays || view.APIUsage[service.ActivityDays-1].Day != "2026-10-17" ||
			view.APIUsage[service.ActivityDays-1].Requests != 4 || view.APIRequests != 4 || view.APIUsage[0].Day != "2026-09-18" {
			t.Fatalf("api usage = %+v total=%d", view.APIUsage[service.ActivityDays-1], view.APIRequests)
		}
		if _, err := activity.Activity(t.Context(), user.ID+100); err == nil {
			t.Fatal("missing account returned activity")
		}
	})
}
