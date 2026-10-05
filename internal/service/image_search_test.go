package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/biliblihuorong/imgnest/internal/model"
	"github.com/biliblihuorong/imgnest/internal/searchquery"
)

type searchAlbumStore struct {
	AlbumStore
	rows  []model.Album
	calls int
}

func (s *searchAlbumStore) SearchAlbums(_ context.Context, owner uint64, id *uint64, name *string, scope *uint64) ([]model.Album, error) {
	s.calls++
	found := []model.Album{}
	for _, a := range s.rows {
		if a.UserID == owner && (scope == nil || a.ID == *scope) && ((id != nil && a.ID == *id) || (name != nil && searchquery.NFC(a.Name) == *name)) {
			found = append(found, a)
		}
	}
	return found, nil
}

func TestSearchServiceResolvesAndLocksScope(t *testing.T) {
	svc, images, _, subject := albumImageFixture(t)
	albums := &searchAlbumStore{rows: []model.Album{{ID: 42, UserID: 1, Name: "暑假照片"}, {ID: 57, UserID: 1, Name: "工作"}, {ID: 99, UserID: 2, Name: "Secret"}, {ID: 70, UserID: 1, Name: "同名"}, {ID: 71, UserID: 1, Name: "同名"}}}
	svc.deps.Albums = albums
	locked := uint64(42)
	result, err := svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Q: `暑假 album:"暑假照片",unfiled after:2026-10-01 before:2026-10-02 maxsize:0MB`, Timezone: "Asia/Shanghai", Page: 1, Size: 20, LockedAlbumID: &locked})
	if err != nil {
		t.Fatal(err)
	}
	if result.Search == nil || result.Search.AppliedVersion != 1 || result.Search.CanonicalQ != `"暑假" album:unfiled,#42 maxsize:0MB after:2026-10-01 before:2026-10-02` || result.Search.AppliedRange.AfterUTC == nil || *result.Search.AppliedRange.AfterUTC != "2026-09-30T16:00:00Z" {
		t.Fatalf("metadata=%+v", result.Search)
	}
	f := images.capturedFilter()
	if f.Admin || f.UserID != 1 || f.AlbumID == nil || *f.AlbumID != 42 || f.Search == nil || !reflect.DeepEqual(f.Search.AlbumIDs, []uint64{42}) || !f.Search.IncludeUnfiled || f.Search.MaxBytes == nil || *f.Search.MaxBytes != 0 {
		t.Fatalf("unsafe/lossy filter=%+v", f)
	}
	for _, v := range []struct{ q, tz, code string }{{`album:#99`, "UTC", "ALBUM_NOT_AVAILABLE"}, {`album:#999`, "UTC", "ALBUM_NOT_AVAILABLE"}, {`album:"Secret"`, "UTC", "ALBUM_NOT_AVAILABLE"}, {`album:"同名"`, "UTC", "ALBUM_AMBIGUOUS"}, {"", "Bad/Zone", "INVALID_TIMEZONE"}, {"after:2011-12-30", "Pacific/Apia", "DATE_NOT_REPRESENTABLE"}, {"foo:bar", "UTC", "UNKNOWN_FIELD"}} {
		_, err := svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Q: v.q, Timezone: v.tz, Page: 1, Size: 20})
		var se *SearchError
		if !errors.As(err, &se) || len(se.Diagnostics) != 1 || se.Diagnostics[0].Code != v.code {
			t.Errorf("%q: %v want=%s", v.q, err, v.code)
		}
	}
	_, err = svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Q: `album:#57`, Timezone: "UTC", Page: 1, Size: 20, LockedAlbumID: &locked})
	var unavailable *SearchError
	if !errors.As(err, &unavailable) || unavailable.Diagnostics[0].Code != "ALBUM_NOT_AVAILABLE" {
		t.Fatalf("out-of-scope owned album should be unavailable: %v", err)
	}
	locked = 70
	if _, err := svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Q: `album:"同名"`, Timezone: "UTC", Page: 1, Size: 20, LockedAlbumID: &locked}); err != nil {
		t.Fatalf("name unique within locked scope: %v", err)
	}

	locked = 99
	if _, err := svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Timezone: "UTC", Page: 1, Size: 20, LockedAlbumID: &locked}); err == nil {
		t.Fatal("foreign locked album accepted")
	}
}

func TestSearchResolvedCanonicalBudget(t *testing.T) {
	svc, images, _, subject := albumImageFixture(t)
	svc.deps.Albums = &searchAlbumStore{rows: []model.Album{{ID: 9223372036854775807, UserID: 1, Name: "a"}}}
	terms := []string{}
	for i := 0; i < 8; i++ {
		n := 126
		if i == 7 {
			n = 130
		}
		terms = append(terms, strings.Repeat(string(rune(0x1f600+i)), n))
	}
	raw := strings.Join(terms, " ") + " album:a"
	if p := searchquery.Parse(raw); !p.OK {
		t.Fatalf("local query should fit: %+v", p.Diagnostics)
	}
	_, err := svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Q: raw, Timezone: "UTC", Page: 1, Size: 20})
	var problem *SearchError
	if !errors.As(err, &problem) || problem.Diagnostics[0].Code != "QUERY_TOO_COMPLEX" {
		t.Fatalf("oversized resolved canonical accepted: %v", err)
	}
	if images.capturedFilter().Search != nil {
		t.Fatal("database image query executed before canonical budget check")
	}
}

func TestSearchAlbumErrorsHighlightQuotedNames(t *testing.T) {
	svc, _, _, subject := albumImageFixture(t)
	svc.deps.Albums = &searchAlbumStore{rows: []model.Album{{ID: 42, UserID: 1, Name: "other"}}}
	for _, v := range []struct {
		q    string
		span searchquery.Span
	}{{`album:#42,"#42"`, searchquery.Span{Start: 11, End: 14}}, {`album:unfiled,"unfiled"`, searchquery.Span{Start: 15, End: 22}}} {
		_, err := svc.List(t.Context(), subject, ImageQuery{QueryVersion: 1, Q: v.q, Timezone: "UTC", Page: 1, Size: 20})
		var p *SearchError
		if !errors.As(err, &p) || p.Diagnostics[0].Code != "ALBUM_NOT_AVAILABLE" || p.Diagnostics[0].Span != v.span {
			t.Fatalf("%q diagnostic=%+v want span=%+v", v.q, p, v.span)
		}
	}
}
