package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/searchquery"
)

// AuthorizedAlbum is the minimal, current-owner album identity metadata.
type AuthorizedAlbum struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// AppliedRange exposes the authoritative half-open UTC date boundaries.
type AppliedRange struct {
	AfterUTC  *string `json:"afterUtc"`
	BeforeUTC *string `json:"beforeUtc"`
}

// SearchMetadata confirms which version and normalized query were executed.
type SearchMetadata struct {
	AppliedVersion   int               `json:"appliedVersion"`
	CanonicalQ       string            `json:"canonicalQ"`
	Timezone         string            `json:"tz"`
	AuthorizedAlbums []AuthorizedAlbum `json:"authorizedAlbums"`
	AppliedRange     AppliedRange      `json:"appliedRange"`
}

// SearchError carries stable diagnostics, never a partial successful result.
type SearchError struct {
	Diagnostics []searchquery.Diagnostic `json:"diagnostics"`
}

func (e *SearchError) Error() string { return "invalid image search" }
func (e *SearchError) Unwrap() error { return ErrInvalidInput }

// SearchDiagnostic constructs one stable query or parameter diagnostic.
func SearchDiagnostic(code string, span searchquery.Span, args map[string]any) *SearchError {
	return &SearchError{Diagnostics: []searchquery.Diagnostic{searchquery.NewDiagnostic(code, span, args)}}
}

// Status distinguishes unavailable/ambiguous albums from syntax errors.
func (e *SearchError) Status() int {
	for _, d := range e.Diagnostics {
		if d.Code == "ALBUM_NOT_AVAILABLE" || d.Code == "ALBUM_AMBIGUOUS" {
			return 422
		}
	}
	return 400
}

// SearchAlbumRepository resolves minimal candidates within the authenticated scope.
type SearchAlbumRepository interface {
	SearchAlbums(context.Context, uint64, *uint64, *string, *uint64) ([]model.Album, error)
}

// AlbumSuggestionRepository pages minimal owner-only suggestions.
type AlbumSuggestionRepository interface {
	SearchAlbumRepository
	SuggestAlbums(context.Context, uint64, string, int, int, *uint64) ([]model.Album, bool, error)
}

// AlbumSuggestions exposes no counts, covers, descriptions, or private metadata.
type AlbumSuggestions struct {
	Items   []AuthorizedAlbum `json:"items"`
	HasMore bool              `json:"hasMore"`
}

func (s *ImageService) listSearch(ctx context.Context, subject TokenSubject, query ImageQuery) (ImagePage, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	actor, err := s.actor(ctx, subject)
	if err != nil {
		return ImagePage{}, err
	}
	if query.Admin || query.Trash {
		return ImagePage{}, ErrForbidden
	}
	if query.QueryVersion != 1 {
		return ImagePage{}, SearchDiagnostic("UNSUPPORTED_QUERY_VERSION", searchquery.Span{}, nil)
	}
	if query.AlbumID != nil || query.Keyword != "" || query.Order != "" || query.Exif != "" || query.MinSize != 0 || query.MaxSize != 0 || query.From != nil || query.To != nil {
		return ImagePage{}, SearchDiagnostic("MIXED_QUERY_PROTOCOL", searchquery.Span{}, nil)
	}
	if query.Page < 1 || query.Page > 10000 || (query.Size != 20 && query.Size != 50 && query.Size != 100) {
		return ImagePage{}, SearchDiagnostic("INVALID_PAGINATION", searchquery.Span{}, nil)
	}
	albums, hasAlbums := s.deps.Albums.(SearchAlbumRepository)
	authorized := map[uint64]string{}
	// The route ID is a server-verified fixed boundary, never part of client q.
	if query.LockedAlbumID != nil {
		if !hasAlbums {
			return ImagePage{}, ErrStorage
		}
		found, err := albums.SearchAlbums(ctx, actor.ID, query.LockedAlbumID, nil, nil)
		if err != nil {
			return ImagePage{}, err
		}
		if len(found) != 1 {
			return ImagePage{}, SearchDiagnostic("ALBUM_NOT_AVAILABLE", searchquery.Span{}, nil)
		}
		authorized[found[0].ID] = found[0].Name
	}
	parsed := searchquery.Parse(query.Q)
	if !parsed.OK {
		return ImagePage{}, &SearchError{Diagnostics: parsed.Diagnostics}
	}
	ast := *parsed.AST
	loc, err := searchquery.Location(query.Timezone)
	if err != nil {
		return ImagePage{}, SearchDiagnostic("INVALID_TIMEZONE", searchquery.Span{}, nil)
	}
	filter := model.ImageSearchFilterFromAST(ast)
	ids := map[uint64]bool{}
	for _, album := range ast.Filters.Albums {
		if album.Kind == "unfiled" {
			filter.IncludeUnfiled = true
			continue
		}
		if !hasAlbums {
			return ImagePage{}, ErrStorage
		}
		sp := albumSpan(parsed.Tokens, album)
		var id *uint64
		var name *string
		if album.Kind == "id" {
			v, e := strconv.ParseUint(album.Value, 10, 64)
			if e != nil || v > math.MaxInt64 {
				return ImagePage{}, SearchDiagnostic("ALBUM_NOT_AVAILABLE", sp, nil)
			}
			id = &v
		} else {
			v := album.Value
			name = &v
		}
		found, e := albums.SearchAlbums(ctx, actor.ID, id, name, query.LockedAlbumID)
		if e != nil {
			return ImagePage{}, fmt.Errorf("resolve search album: %w", e)
		}
		if len(found) == 0 {
			return ImagePage{}, SearchDiagnostic("ALBUM_NOT_AVAILABLE", sp, nil)
		}
		if len(found) > 1 {
			candidates := []AuthorizedAlbum{}
			for _, a := range found[:min(len(found), 5)] {
				candidates = append(candidates, AuthorizedAlbum{strconv.FormatUint(a.ID, 10), a.Name})
			}
			return ImagePage{}, SearchDiagnostic("ALBUM_AMBIGUOUS", sp, map[string]any{"candidates": candidates})
		}
		ids[found[0].ID] = true
		authorized[found[0].ID] = found[0].Name
	}
	count := len(ids)
	if filter.IncludeUnfiled {
		count++
	}
	if count > 5 {
		return ImagePage{}, SearchDiagnostic("QUERY_TOO_COMPLEX", fieldSpan(parsed.Tokens, "album"), nil)
	}
	for id := range ids {
		filter.AlbumIDs = append(filter.AlbumIDs, id)
	}
	sort.Slice(filter.AlbumIDs, func(i, j int) bool { return filter.AlbumIDs[i] < filter.AlbumIDs[j] })
	ast.Filters.Albums = []searchquery.Album{}
	if filter.IncludeUnfiled {
		ast.Filters.Albums = append(ast.Filters.Albums, searchquery.Album{Kind: "unfiled"})
	}
	for _, id := range filter.AlbumIDs {
		ast.Filters.Albums = append(ast.Filters.Albums, searchquery.Album{Kind: "id", Value: strconv.FormatUint(id, 10)})
	}
	metadata := &SearchMetadata{AppliedVersion: 1, CanonicalQ: searchquery.Serialize(ast), Timezone: query.Timezone, AuthorizedAlbums: []AuthorizedAlbum{}}
	if len(metadata.CanonicalQ) > 4096 {
		return ImagePage{}, SearchDiagnostic("QUERY_TOO_COMPLEX", searchquery.Span{Start: 0, End: len(utf16.Encode([]rune(query.Q)))}, nil)
	}
	ordered := []uint64{}
	for id := range authorized {
		ordered = append(ordered, id)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for _, id := range ordered {
		metadata.AuthorizedAlbums = append(metadata.AuthorizedAlbums, AuthorizedAlbum{strconv.FormatUint(id, 10), authorized[id]})
	}
	for _, v := range []struct {
		field  string
		day    *string
		target **time.Time
		wire   **string
	}{{"after", ast.Filters.After, &filter.AfterUTC, &metadata.AppliedRange.AfterUTC}, {"before", ast.Filters.Before, &filter.BeforeUTC, &metadata.AppliedRange.BeforeUTC}} {
		if v.day == nil {
			continue
		}
		at, e := searchquery.DayStart(*v.day, loc)
		if e != nil {
			code := "INVALID_DATE"
			if e.Error() == "DATE_NOT_REPRESENTABLE" {
				code = e.Error()
			}
			return ImagePage{}, SearchDiagnostic(code, fieldSpan(parsed.Tokens, v.field), nil)
		}
		*v.target = &at
		text := at.Format(time.RFC3339)
		*v.wire = &text
	}
	images, total, err := s.deps.Images.List(ctx, model.ImageListFilter{UserID: actor.ID, AlbumID: query.LockedAlbumID, Search: &filter}, query.Page, query.Size)
	if err != nil {
		return ImagePage{}, fmt.Errorf("search images: %w", err)
	}
	views := make([]ImageView, 0, len(images))
	for _, im := range images {
		view, e := s.view(ctx, im)
		if e != nil {
			return ImagePage{}, e
		}
		views = append(views, view)
	}
	return ImagePage{Items: views, Total: total, Page: query.Page, Size: query.Size, Search: metadata}, nil
}
func fieldSpan(tokens []searchquery.Token, field string) searchquery.Span {
	for _, t := range tokens {
		if t.Field == field {
			return t.ValueSpan
		}
	}
	return searchquery.Span{}
}
func albumSpan(tokens []searchquery.Token, album searchquery.Album) searchquery.Span {
	for _, t := range tokens {
		if t.Field != "album" {
			continue
		}
		for _, v := range t.Values {
			if searchquery.AlbumValue(v) == album {
				return v.Span
			}
		}
	}
	return fieldSpan(tokens, "album")
}

// Suggestions is an owner-only, name-only page. Empty keywords are bounded.
func (s *AlbumService) Suggestions(ctx context.Context, owner uint64, keyword string, page, size int, scope *uint64) (AlbumSuggestions, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if owner == 0 {
		return AlbumSuggestions{}, ErrUnauthenticated
	}
	if page < 1 || page > 10000 || size < 1 || size > 20 {
		return AlbumSuggestions{}, SearchDiagnostic("INVALID_PAGINATION", searchquery.Span{}, nil)
	}
	if !utf8.ValidString(keyword) || len(keyword) > 4096 || utf8.RuneCountInString(keyword) > 200 {
		return AlbumSuggestions{}, SearchDiagnostic("QUERY_TOO_COMPLEX", searchquery.Span{}, nil)
	}
	for _, r := range keyword {
		if unicode.IsControl(r) {
			return AlbumSuggestions{}, SearchDiagnostic("CONTROL_CHARACTER", searchquery.Span{}, nil)
		}
	}
	albums, ok := s.albums.(AlbumSuggestionRepository)
	if !ok {
		return AlbumSuggestions{}, ErrStorage
	}
	if scope != nil {
		found, err := albums.SearchAlbums(ctx, owner, scope, nil, nil)
		if err != nil {
			return AlbumSuggestions{}, err
		}
		if len(found) != 1 {
			return AlbumSuggestions{}, SearchDiagnostic("ALBUM_NOT_AVAILABLE", searchquery.Span{}, nil)
		}
	}
	rows, more, err := albums.SuggestAlbums(ctx, owner, searchquery.Normalize(keyword), page, size, scope)
	if err != nil {
		return AlbumSuggestions{}, err
	}
	result := AlbumSuggestions{Items: []AuthorizedAlbum{}, HasMore: more}
	for _, a := range rows {
		result.Items = append(result.Items, AuthorizedAlbum{strconv.FormatUint(a.ID, 10), a.Name})
	}
	return result, nil
}

// IsSearchError extracts structured errors while keeping the generic handlers unchanged.
func IsSearchError(err error) (*SearchError, bool) {
	var value *SearchError
	ok := errors.As(err, &value)
	return value, ok
}
