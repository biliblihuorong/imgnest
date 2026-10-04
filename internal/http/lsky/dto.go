package lsky

import (
	"math"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/biliblihuorong/imgnest/internal/service"
	"github.com/gin-gonic/gin"
)

// kilobytes converts stored bytes into the v1 KB float unit.
func kilobytes(bytes int64) float64 {
	if bytes == 0 {
		return 0
	}
	return float64(bytes) / 1024
}

// albumRef is the album object embedded in a v1 image item.
type albumRef struct {
	ID   uint64 `json:"id"`
	Name string `json:"name"`
}

// imageLinks carries the six link fields every v1 image carries.
type imageLinks struct {
	URL              string `json:"url"`
	HTML             string `json:"html"`
	BBCode           string `json:"bbcode"`
	Markdown         string `json:"markdown"`
	MarkdownWithLink string `json:"markdown_with_link"`
	ThumbnailURL     string `json:"thumbnail_url"`
}

// uploadLinks adds the ImgNest extra fields present on the upload response.
type uploadLinks struct {
	imageLinks
	OriginURL string `json:"origin_url"`
	WebPURL   string `json:"webp_url"`
}

// imageItem is one v1 image entry; field names follow the Lsky contract.
type imageItem struct {
	Album      any        `json:"album"`
	Key        string     `json:"key"`
	Name       string     `json:"name"`
	Pathname   string     `json:"pathname"`
	OriginName string     `json:"origin_name"`
	Size       float64    `json:"size"`
	Mimetype   string     `json:"mimetype"`
	Extension  string     `json:"extension"`
	MD5        string     `json:"md5"`
	SHA1       string     `json:"sha1"`
	Width      int        `json:"width"`
	Height     int        `json:"height"`
	Links      imageLinks `json:"links"`
	HumanDate  string     `json:"human_date"`
	Date       string     `json:"date"`
}

// v1Name is the stored file name with extension.
func v1Name(view service.ImageView) string {
	return path.Base(view.Path) + "." + view.Ext
}

// v1Pathname is the storage path including the file name.
func v1Pathname(view service.ImageView) string {
	return view.Path + "." + view.Ext
}

func buildLinks(view service.ImageView) imageLinks {
	links := imageLinks{URL: view.Links.URL}
	links.HTML = v1Escape("<img src=\"" + links.URL + "\" alt=\"" + view.Name + "\" title=\"" + view.Name + "\" />")
	links.BBCode = "[img]" + links.URL + "[/img]"
	links.Markdown = "![" + view.Name + "](" + links.URL + ")"
	links.MarkdownWithLink = "[![" + view.Name + "](" + links.URL + ")](" + links.URL + ")"
	links.ThumbnailURL = view.Links.Thumbnail
	if links.ThumbnailURL == "" {
		links.ThumbnailURL = links.URL
	}
	return links
}

// v1Escape escapes exactly like the Lsky contract: only &, < and > become
// entities, while the quotes of the img tag stay literal
// (`&lt;img src="URL" alt="NAME" title="NAME" /&gt;`).
func v1Escape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

// buildImageItem maps one image view to its v1 shape without EXIF or GPS.
func buildImageItem(view service.ImageView, album any, now time.Time) imageItem {
	created := view.CreatedAt
	return imageItem{
		Album:      album,
		Key:        view.Key,
		Name:       v1Name(view),
		Pathname:   v1Pathname(view),
		OriginName: view.Name,
		Size:       kilobytes(view.Size),
		Mimetype:   view.MIME,
		Extension:  view.Ext,
		MD5:        view.MD5,
		SHA1:       view.SHA1,
		Width:      view.Width,
		Height:     view.Height,
		Links:      buildLinks(view),
		HumanDate:  humanDate(now, created),
		Date:       created.UTC().Format("2006-01-02 15:04:05"),
	}
}

// buildUploadData maps a fresh upload to the v1 upload response payload.
func buildUploadData(view service.ImageView) gin.H {
	links := uploadLinks{imageLinks: buildLinks(view), OriginURL: view.Links.Original, WebPURL: view.Links.WebP}
	return gin.H{
		"key":         view.Key,
		"name":        v1Name(view),
		"pathname":    v1Pathname(view),
		"origin_name": view.Name,
		"size":        kilobytes(view.Size),
		"mimetype":    view.MIME,
		"extension":   view.Ext,
		"md5":         view.MD5,
		"sha1":        view.SHA1,
		"links":       links,
	}
}

// humanDate renders the Chinese relative time shown in the v1 list.
func humanDate(now, created time.Time) string {
	difference := now.Sub(created)
	switch {
	case difference < time.Minute:
		return "刚刚"
	case difference < time.Hour:
		return strconv.Itoa(int(difference.Minutes())) + " 分钟前"
	case difference < 24*time.Hour:
		return strconv.Itoa(int(difference.Hours())) + " 小时前"
	case difference < 30*24*time.Hour:
		return strconv.Itoa(int(difference.Hours()/24)) + " 天前"
	default:
		return created.Format("2006-01-02")
	}
}

// paginator mirrors the Laravel paginator shape the v1 lists must keep.
type paginator struct {
	CurrentPage  int    `json:"current_page"`
	Data         any    `json:"data"`
	FirstPageURL string `json:"first_page_url"`
	From         any    `json:"from"`
	LastPage     int    `json:"last_page"`
	LastPageURL  string `json:"last_page_url"`
	Links        []any  `json:"links"`
	NextPageURL  any    `json:"next_page_url"`
	Path         string `json:"path"`
	PerPage      int    `json:"per_page"`
	PrevPageURL  any    `json:"prev_page_url"`
	To           any    `json:"to"`
	Total        int64  `json:"total"`
}

// buildPaginator assembles the Laravel paginator over already-mapped items.
func buildPaginator(base string, page, perPage int, total int64, data any) paginator {
	lastPage := int(math.Ceil(float64(total) / float64(perPage)))
	if lastPage < 1 {
		lastPage = 1
	}
	result := paginator{
		CurrentPage: page, Data: data,
		FirstPageURL: pageURL(base, 1), LastPage: lastPage,
		LastPageURL: pageURL(base, lastPage), Links: []any{},
		Path: base, PerPage: perPage, Total: total,
	}
	if page < lastPage {
		result.NextPageURL = pageURL(base, page+1)
	}
	if page > 1 {
		result.PrevPageURL = pageURL(base, page-1)
	}
	if count := itemLen(data); count > 0 {
		from := (page-1)*perPage + 1
		result.From = from
		result.To = from + count - 1
	}
	return result
}

func pageURL(base string, page int) string {
	return base + "?page=" + strconv.Itoa(page)
}

func itemLen(data any) int {
	switch items := data.(type) {
	case []imageItem:
		return len(items)
	case []albumItem:
		return len(items)
	default:
		return 0
	}
}

// albumItem is one v1 album entry.
type albumItem struct {
	ID       uint64 `json:"id"`
	Name     string `json:"name"`
	Intro    string `json:"intro"`
	ImageNum int64  `json:"image_num"`
}

// baseURL rebuilds the absolute origin for paginator URLs.
func baseURL(host string, forwardedProto string, tls bool) string {
	scheme := "http"
	if tls || forwardedProto == "https" {
		scheme = "https"
	}
	return scheme + "://" + host
}
