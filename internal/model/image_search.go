package model

import (
	"github.com/biliblihuorong/imgnest/internal/searchquery"
	"strconv"
	"time"
)

// ImageSearchFilter is an independently authorized, normalized query plan.
// Owner/state and the locked album boundary remain in ImageListFilter.
type ImageSearchFilter struct {
	Terms, Formats      []string
	AlbumIDs            []uint64
	IncludeUnfiled      bool
	Camera              *string
	MinBytes, MaxBytes  *int64
	AfterUTC, BeforeUTC *time.Time
	Visibility, Sort    string
}

// ImageSearchFilterFromAST copies validated scalar predicates; the service
// separately resolves albums and dates against its authenticated scope.
func ImageSearchFilterFromAST(ast searchquery.AST) ImageSearchFilter {
	q := ImageSearchFilter{Terms: ast.Terms, Formats: ast.Filters.Formats, Camera: ast.Filters.Camera, Visibility: ast.Filters.Visibility, Sort: ast.Sort}
	if ast.Filters.MinBytes != nil {
		n, _ := strconv.ParseInt(*ast.Filters.MinBytes, 10, 64)
		q.MinBytes = &n
	}
	if ast.Filters.MaxBytes != nil {
		n, _ := strconv.ParseInt(*ast.Filters.MaxBytes, 10, 64)
		q.MaxBytes = &n
	}
	return q
}
