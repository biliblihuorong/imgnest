package native

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strings"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// Crawler answers robots.txt and the image sitemap.
type Crawler interface {
	Robots(ctx context.Context, origin string) string
	Sitemap(ctx context.Context) (service.Sitemap, error)
}

// CrawlerOptions configures the robots.txt and sitemap routes.
type CrawlerOptions struct {
	// TrustedProxies are the addresses or CIDRs whose X-Forwarded-Proto
	// decides the scheme of the absolute URLs written into the responses.
	TrustedProxies []string
}

// crawlerCacheControl lets crawlers and CDNs reuse the answers for a while;
// a settings change reaches them within the hour.
const crawlerCacheControl = "public, max-age=3600"

// RegisterCrawlerRoutes binds /robots.txt and /sitemap.xml.
func (h *Handler) RegisterCrawlerRoutes(ctx context.Context, router gin.IRouter, crawler Crawler, options CrawlerOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if crawler == nil {
		return service.ErrInvalidInput
	}
	proxies, err := parseTrustedProxies(options.TrustedProxies)
	if err != nil {
		return fmt.Errorf("register crawler routes: %w", err)
	}
	handler := &crawlerHandler{crawler: crawler, proxies: proxies}
	router.GET("/robots.txt", handler.robots)
	router.HEAD("/robots.txt", handler.robots)
	router.GET("/sitemap.xml", handler.sitemap)
	router.HEAD("/sitemap.xml", handler.sitemap)
	return nil
}

type crawlerHandler struct {
	crawler Crawler
	proxies []netip.Prefix
}

func (h *crawlerHandler) robots(c *gin.Context) {
	c.Header("Cache-Control", crawlerCacheControl)
	c.Data(http.StatusOK, "text/plain; charset=utf-8", []byte(h.crawler.Robots(c.Request.Context(), h.origin(c))))
}

func (h *crawlerHandler) sitemap(c *gin.Context) {
	sitemap, err := h.crawler.Sitemap(c.Request.Context())
	if err != nil {
		if !errors.Is(err, service.ErrNotFound) {
			_ = c.Error(err)
		}
		// A switched-off sitemap and a failed read look the same, so the
		// route reveals nothing about the settings behind it.
		c.Header("X-Robots-Tag", "noindex")
		c.Data(http.StatusNotFound, "text/plain; charset=utf-8", []byte("not found\n"))
		return
	}
	origin := h.origin(c)
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9" xmlns:image="http://www.google.com/schemas/sitemap-image/1.1">` + "\n")
	b.WriteString("  <url><loc>" + xmlEscape(origin+"/home") + "</loc></url>\n")
	b.WriteString("  <url>\n    <loc>" + xmlEscape(origin+"/gallery") + "</loc>\n")
	for _, link := range sitemap.ImageURLs {
		if strings.HasPrefix(link, "/") {
			link = origin + link
		}
		b.WriteString("    <image:image><image:loc>" + xmlEscape(link) + "</image:loc></image:image>\n")
	}
	b.WriteString("  </url>\n</urlset>\n")
	c.Header("Cache-Control", crawlerCacheControl)
	c.Data(http.StatusOK, "application/xml; charset=utf-8", []byte(b.String()))
}

// origin rebuilds the scheme and host the client used. X-Forwarded-Proto
// counts only from a configured trusted proxy.
func (h *crawlerHandler) origin(c *gin.Context) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	} else if peer, err := netip.ParseAddr(c.RemoteIP()); err == nil {
		peer = peer.Unmap()
		for _, prefix := range h.proxies {
			if prefix.Contains(peer) && c.GetHeader("X-Forwarded-Proto") == "https" {
				scheme = "https"
				break
			}
		}
	}
	return scheme + "://" + c.Request.Host
}

var xmlReplacer = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")

func xmlEscape(value string) string {
	return xmlReplacer.Replace(value)
}

func parseTrustedProxies(values []string) ([]netip.Prefix, error) {
	proxies := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		if addr, err := netip.ParseAddr(value); err == nil {
			addr = addr.Unmap()
			proxies = append(proxies, netip.PrefixFrom(addr, addr.BitLen()))
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, errors.New("invalid trusted proxy")
		}
		proxies = append(proxies, prefix.Masked())
	}
	return proxies, nil
}
