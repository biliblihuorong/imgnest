package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/biliblihuorong/imgnest/internal/model"
)

// ActivityRepository reads the recorded sign-ins and API usage of an account.
type ActivityRepository interface {
	ListTokens(ctx context.Context, userID uint64) ([]model.Token, error)
	ListLoginEvents(ctx context.Context, userID uint64, limit int) ([]model.LoginEvent, error)
	ListAPIUsage(ctx context.Context, userID uint64, since time.Time) ([]model.APIUsageDay, error)
}

// ActivityDays is the length of the daily API usage series.
const ActivityDays = 30

// activityLogins is how many recent sign-ins an activity view lists.
const activityLogins = 10

// LoginView is one recorded sign-in.
type LoginView struct {
	At        time.Time `json:"at"`
	IP        string    `json:"ip"`
	UserAgent string    `json:"user_agent"`
}

// UsageDayView is one UTC day of API-token requests.
type UsageDayView struct {
	Day      string `json:"day"`
	Requests int64  `json:"requests"`
}

// ActivityView summarizes how an account has been used. It is returned to
// the account owner and to administrators only.
type ActivityView struct {
	LastSeenAt *time.Time `json:"last_seen_at"`
	// LastLogin is the newest sign-in, or null when none was recorded.
	LastLogin *LoginView  `json:"last_login"`
	Logins    []LoginView `json:"logins"`
	// ActiveSessions counts unexpired web sign-ins.
	ActiveSessions int `json:"active_sessions"`
	// Tokens lists the account's API tokens with their usage.
	Tokens []TokenView `json:"tokens"`
	// APIUsage holds one entry per UTC day, oldest first, ending today.
	APIUsage    []UsageDayView `json:"api_usage"`
	APIRequests int64          `json:"api_requests"`
}

// ActivityService assembles account activity for owners and administrators.
type ActivityService struct {
	users    UserRepository
	activity ActivityRepository
	now      func() time.Time
}

// NewActivityService constructs activity reads.
func NewActivityService(ctx context.Context, users UserRepository, activity ActivityRepository, now func() time.Time) (*ActivityService, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("construct activity service: %w", err)
	}
	if users == nil || activity == nil || now == nil {
		return nil, fmt.Errorf("activity service dependencies: %w", ErrInvalidInput)
	}
	return &ActivityService{users: users, activity: activity, now: now}, nil
}

// Activity returns one account's activity. Callers authorize the reader:
// the account itself or an administrator.
func (s *ActivityService) Activity(ctx context.Context, userID uint64) (ActivityView, error) {
	if err := ctx.Err(); err != nil {
		return ActivityView{}, fmt.Errorf("read activity: %w", err)
	}
	if userID == 0 {
		return ActivityView{}, ErrInvalidInput
	}
	user, err := s.users.FindUserByID(ctx, userID)
	if errors.Is(err, ErrNotFound) {
		return ActivityView{}, ErrNotFound
	}
	if err != nil {
		return ActivityView{}, fmt.Errorf("find activity user: %w", err)
	}
	now := s.now().UTC()
	view := ActivityView{LastSeenAt: utcTime(user.LastSeenAt), Logins: []LoginView{}, Tokens: []TokenView{}}
	events, err := s.activity.ListLoginEvents(ctx, user.ID, activityLogins)
	if err != nil {
		return ActivityView{}, fmt.Errorf("list login events: %w", err)
	}
	for _, event := range events {
		view.Logins = append(view.Logins, LoginView{At: event.CreatedAt.UTC(), IP: event.IP, UserAgent: event.UserAgent})
	}
	if len(view.Logins) > 0 {
		last := view.Logins[0]
		view.LastLogin = &last
	}
	tokens, err := s.activity.ListTokens(ctx, user.ID)
	if err != nil {
		return ActivityView{}, fmt.Errorf("list activity tokens: %w", err)
	}
	for _, token := range tokens {
		expired := token.ExpiresAt != nil && !token.ExpiresAt.After(now)
		switch {
		case token.Kind == TokenKindWeb && !expired:
			view.ActiveSessions++
		case token.Kind == TokenKindAPI:
			view.Tokens = append(view.Tokens, tokenView(token))
		}
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	since := today.AddDate(0, 0, 1-ActivityDays)
	usage, err := s.activity.ListAPIUsage(ctx, user.ID, since)
	if err != nil {
		return ActivityView{}, fmt.Errorf("list api usage: %w", err)
	}
	byDay := make(map[string]int64, len(usage))
	for _, day := range usage {
		byDay[day.Day] += day.Requests
	}
	view.APIUsage = make([]UsageDayView, 0, ActivityDays)
	for i := range ActivityDays {
		day := since.AddDate(0, 0, i).Format(time.DateOnly)
		view.APIUsage = append(view.APIUsage, UsageDayView{Day: day, Requests: byDay[day]})
		view.APIRequests += byDay[day]
	}
	return view, nil
}
