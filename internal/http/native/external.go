package native

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/biliblihuorong/imgnest/extension"
	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// ExternalSignIn resolves identities verified by plugins to accounts.
type ExternalSignIn interface {
	SignInExternal(context.Context, service.ExternalSignIn) (service.VerifiedCredentials, error)
}

// Sign-in tickets carry a finished external sign-in from the plugin's
// redirect to the web app, which trades one for a session token. They live
// in memory: ImgNest runs as a single instance.
const (
	ticketTTL  = 2 * time.Minute
	maxTickets = 4096
	// ssoLanding receives the ticket in the URL fragment, which browsers
	// never send to servers or in Referer headers.
	ssoLanding = "/auth/sso"
)

type ticketEntry struct {
	credentials service.VerifiedCredentials
	expires     time.Time
}

type ticketStore struct {
	mu    sync.Mutex
	now   func() time.Time
	items map[string]ticketEntry
}

func newTicketStore(now func() time.Time) *ticketStore {
	return &ticketStore{now: now, items: map[string]ticketEntry{}}
}

func (s *ticketStore) put(credentials service.VerifiedCredentials) (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	ticket := base64.RawURLEncoding.EncodeToString(raw)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	for key, entry := range s.items {
		if !now.Before(entry.expires) {
			delete(s.items, key)
		}
	}
	if len(s.items) >= maxTickets {
		return "", errors.New("too many pending sign-ins")
	}
	s.items[ticket] = ticketEntry{credentials: credentials, expires: now.Add(ticketTTL)}
	return ticket, nil
}

// take consumes a ticket; each one works once.
func (s *ticketStore) take(ticket string) (service.VerifiedCredentials, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.items[ticket]
	delete(s.items, ticket)
	if !ok || !s.now().Before(entry.expires) {
		return service.VerifiedCredentials{}, false
	}
	return entry.credentials, true
}

// pluginHost is the extension.Host handed to one plugin; it namespaces that
// plugin's provider IDs so two plugins can never claim each other's subjects.
type pluginHost struct {
	handler *Handler
	plugin  string
}

// Host returns the sign-in host for the named plugin.
func (h *Handler) Host(plugin string) extension.Host { return pluginHost{handler: h, plugin: plugin} }

func (p pluginHost) CompleteSignIn(c *gin.Context, identity extension.ExternalIdentity) {
	h := p.handler
	if h.signIn == nil {
		redirectSignInError(c, "unavailable")
		return
	}
	credentials, err := h.signIn.SignInExternal(c.Request.Context(), service.ExternalSignIn{
		Provider: p.plugin + ":" + identity.Provider, Subject: identity.Subject,
		Email: identity.Email, EmailVerified: identity.EmailVerified,
		UsernameHint: identity.Username, LinkByEmail: identity.LinkByEmail, IP: c.ClientIP(),
	})
	if err != nil {
		redirectSignInError(c, signInErrorCode(err))
		return
	}
	ticket, err := h.tickets.put(credentials)
	if err != nil {
		redirectSignInError(c, "failed")
		return
	}
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusSeeOther, ssoLanding+"#ticket="+ticket)
}

func (p pluginHost) FailSignIn(c *gin.Context) { redirectSignInError(c, "failed") }

func signInErrorCode(err error) string {
	switch {
	case errors.Is(err, service.ErrIdentityNotLinked):
		return "not_linked"
	case errors.Is(err, service.ErrIdentityEmailRequired):
		return "email_required"
	case errors.Is(err, service.ErrForbidden):
		return "disabled"
	default:
		return "failed"
	}
}

func redirectSignInError(c *gin.Context, code string) {
	c.Header("Referrer-Policy", "no-referrer")
	c.Redirect(http.StatusSeeOther, "/login?sso_error="+code)
}

// exchangeTicket trades a one-time sign-in ticket for a web session, with the
// same response shape as the password login.
func (h *Handler) exchangeTicket(c *gin.Context) {
	var in struct {
		Ticket string `json:"ticket"`
	}
	if !decode(c, &in) {
		return
	}
	credentials, ok := h.tickets.take(in.Ticket)
	if !ok {
		fail(c, service.ErrUnauthenticated)
		return
	}
	issued, err := h.tokens.Issue(c.Request.Context(), credentials.Subject, service.TokenInput{Name: "web", Kind: "web", Abilities: []string{"*"}})
	if err != nil {
		fail(c, err)
		return
	}
	respond(c, 200, gin.H{"token": issued.Token, "user": credentials.User, "expires_at": issued.Info.ExpiresAt})
}
