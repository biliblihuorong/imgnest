package service

import (
	"context"
	"strings"
	"unicode/utf8"
)

// ClientInfo describes the HTTP client behind a request, recorded with
// sign-ins and token use.
type ClientInfo struct {
	IP        string
	UserAgent string
}

type clientKey struct{}

// maxUserAgentBytes bounds the stored User-Agent header.
const maxUserAgentBytes = 255

// WithClient attaches the request's client details to ctx.
func WithClient(ctx context.Context, client ClientInfo) context.Context {
	client.IP = strings.TrimSpace(client.IP)
	client.UserAgent = truncateUTF8(strings.TrimSpace(client.UserAgent), maxUserAgentBytes)
	return context.WithValue(ctx, clientKey{}, client)
}

// ClientFrom returns the client details attached by WithClient, or zero values.
func ClientFrom(ctx context.Context) ClientInfo {
	client, _ := ctx.Value(clientKey{}).(ClientInfo)
	return client
}

func truncateUTF8(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	value = value[:limit]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
