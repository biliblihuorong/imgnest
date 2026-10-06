// Package searchquery implements the versioned, pure ImageNest search grammar.
// It never reads accounts, databases, files, or the clock while parsing.
package searchquery

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// Formats is the canonical, ordered actual-format enumeration.
var Formats = []string{"jpg", "png", "gif", "webp", "avif", "bmp", "tiff", "svg", "heic", "heif", "unknown"}

// Span is a half-open JavaScript UTF-16 source range.
type Span struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Diagnostic is a localizable, non-executable parse failure.
type Diagnostic struct {
	Code       string         `json:"code"`
	MessageKey string         `json:"messageKey"`
	Span       Span           `json:"span"`
	Args       map[string]any `json:"args"`
}

// Album is an unresolved name, decimal-string ID, or unfiled marker.
type Album struct {
	Kind  string `json:"kind"`
	Value string `json:"value,omitempty"`
}

// Filters contains the fixed version-one filter keys and explicit defaults.
type Filters struct {
	Formats    []string `json:"formats"`
	Albums     []Album  `json:"albums"`
	Camera     *string  `json:"camera"`
	MinBytes   *string  `json:"minBytes"`
	MaxBytes   *string  `json:"maxBytes"`
	After      *string  `json:"after"`
	Before     *string  `json:"before"`
	Visibility string   `json:"visibility"`
}

// AST is the complete pure version-one query tree.
type AST struct {
	Version int      `json:"version"`
	Terms   []string `json:"terms"`
	Filters Filters  `json:"filters"`
	Sort    string   `json:"sort"`
}

// Value keeps decoded text and its original source range outside the AST.
type Value struct {
	Text   string `json:"text"`
	Quoted bool   `json:"quoted"`
	Span   Span   `json:"span"`
}

// Token identifies one source token and its canonical field name.
type Token struct {
	Field     string  `json:"field"`
	Span      Span    `json:"span"`
	ValueSpan Span    `json:"valueSpan"`
	Values    []Value `json:"values"`
}

// Result contains either a complete AST or diagnostics, never both.
type Result struct {
	OK          bool         `json:"ok"`
	AST         *AST         `json:"ast,omitempty"`
	Tokens      []Token      `json:"tokens,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
}

// Normalize folds ASCII only after NFC, deliberately preserving accents and non-ASCII case.
func Normalize(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + 32
		}
		return r
	}, norm.NFC.String(s))
}

// NFC normalizes text without any case folding.
func NFC(s string) string { return norm.NFC.String(s) }

// NewDiagnostic maps a stable code to its client localization key.
func NewDiagnostic(code string, span Span, args map[string]any) Diagnostic {
	parts := strings.Split(strings.ToLower(code), "_")
	key := parts[0]
	for _, p := range parts[1:] {
		key += strings.ToUpper(p[:1]) + p[1:]
	}
	if args == nil {
		args = map[string]any{}
	}
	return Diagnostic{Code: code, MessageKey: "search.error." + key, Span: span, Args: args}
}
func utf16len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xffff {
			n++
		}
	}
	return n
}
func span(raw string, start, end int) Span { return Span{utf16len(raw[:start]), utf16len(raw[:end])} }
func whitespace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n' || r == '\v' || r == '\f' || r == '\u3000' || r == '\u00a0'
}
func fail(d Diagnostic) Result { return Result{OK: false, Diagnostics: []Diagnostic{d}} }

type piece struct{ start, end int }

func scan(raw string) ([]piece, *Diagnostic) {
	if len(raw) > 4096 {
		d := NewDiagnostic("QUERY_TOO_COMPLEX", span(raw, 0, len(raw)), nil)
		return nil, &d
	}
	if !utf8.ValidString(raw) {
		d := NewDiagnostic("UNEXPECTED_CHARACTER", Span{0, utf16len(raw)}, nil)
		return nil, &d
	}
	pieces := []piece{}
	start := -1
	quoted := false
	escaped := false
	for i, r := range raw {
		if unicode.IsControl(r) && (!whitespace(r) || quoted) {
			d := NewDiagnostic("CONTROL_CHARACTER", span(raw, i, i+utf8.RuneLen(r)), nil)
			return nil, &d
		}
		if !quoted && whitespace(r) {
			if start >= 0 {
				pieces = append(pieces, piece{start, i})
				start = -1
			}
			continue
		}
		if start < 0 {
			start = i
		}
		if escaped {
			escaped = false
			continue
		}
		if quoted && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			quoted = !quoted
		}
	}
	if start >= 0 {
		pieces = append(pieces, piece{start, len(raw)})
	}
	if len(pieces) > 32 {
		d := NewDiagnostic("QUERY_TOO_COMPLEX", span(raw, pieces[32].start, pieces[32].end), nil)
		return nil, &d
	}
	return pieces, nil
}

var fieldName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)
var numericID = regexp.MustCompile(`^#[0-9]+$`)
var idText = regexp.MustCompile(`^#[1-9][0-9]*$`)
var sizeText = regexp.MustCompile(`^([0-9]+)(?:\.([0-9]{1,6}))?[mM][bB]$`)
var dateText = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)

// Parse returns either a complete executable AST, or diagnostics with UTF-16 spans.
func Parse(raw string) Result {
	pieces, d := scan(raw)
	if d != nil {
		return fail(*d)
	}
	ast := AST{Version: 1, Terms: []string{}, Filters: Filters{Formats: []string{}, Albums: []Album{}, Visibility: "all"}, Sort: "newest"}
	tokens := []Token{}
	seen := map[string]Token{}
	terms := map[string]bool{}
	formats := map[string]bool{}
	albums := map[string]bool{}
	rawTerms, rawAlbums := 0, 0
	for _, p := range pieces {
		text := raw[p.start:p.end]
		field := "term"
		inputField := "term"
		valueStart := p.start
		if text[0] != '"' {
			if colon := strings.IndexByte(text, ':'); colon >= 0 {
				inputField = Normalize(text[:colon])
				field = inputField
				valueStart = p.start + colon + 1
				if !fieldName.MatchString(text[:colon]) {
					return fail(NewDiagnostic("UNEXPECTED_CHARACTER", span(raw, p.start, p.start+colon), nil))
				}
				switch field {
				case "extension":
					field = "format"
				case "is":
					field = "visibility"
				case "order":
					field = "sort"
				case "format", "album", "camera", "minsize", "maxsize", "after", "before", "visibility", "sort":
				default:
					return fail(NewDiagnostic("UNKNOWN_FIELD", span(raw, p.start, p.start+colon), map[string]any{"field": inputField}))
				}
			} else if colon := strings.IndexRune(text, '：'); colon >= 0 && fieldName.MatchString(text[:colon]) {
				return fail(NewDiagnostic("UNEXPECTED_CHARACTER", span(raw, p.start+colon, p.start+colon+len("：")), map[string]any{"suggestion": ":"}))
			}
		}
		token := Token{Field: field, Span: span(raw, p.start, p.end), ValueSpan: span(raw, valueStart, p.end)}
		if field != "term" && field != "format" && field != "album" {
			if _, exists := seen[field]; exists {
				return fail(NewDiagnostic("DUPLICATE_FIELD", token.Span, map[string]any{"field": field}))
			}
		}
		values, diag := parseValues(raw, valueStart, p.end, field == "format" || field == "album", field == "term")
		if diag != nil {
			return fail(*diag)
		}
		token.Values = values
		seen[field] = token
		tokens = append(tokens, token)
		for _, v := range values {
			if utf8.RuneCountInString(v.Text) > 200 {
				return fail(NewDiagnostic("QUERY_TOO_COMPLEX", v.Span, nil))
			}
			switch field {
			case "term":
				rawTerms++
				if rawTerms > 16 {
					return fail(NewDiagnostic("QUERY_TOO_COMPLEX", token.Span, nil))
				}
				term := Normalize(v.Text)
				if !terms[term] {
					ast.Terms = append(ast.Terms, term)
					terms[term] = true
				}
				if len(ast.Terms) > 8 {
					return fail(NewDiagnostic("QUERY_TOO_COMPLEX", token.Span, nil))
				}
			case "format":
				f := Normalize(v.Text)
				if f == "jpeg" {
					f = "jpg"
				}
				if f == "tif" {
					f = "tiff"
				}
				if !contains(Formats, f) {
					return fail(NewDiagnostic("INVALID_ENUM", v.Span, map[string]any{"field": field}))
				}
				formats[f] = true
			case "album":
				rawAlbums++
				if rawAlbums > 10 {
					return fail(NewDiagnostic("QUERY_TOO_COMPLEX", v.Span, nil))
				}
				if !v.Quoted && numericID.MatchString(v.Text) && !idText.MatchString(v.Text) {
					return fail(NewDiagnostic("INVALID_ENUM", v.Span, map[string]any{"field": field}))
				}
				a := AlbumValue(v)
				k := a.Kind + ":" + a.Value
				if !albums[k] {
					ast.Filters.Albums = append(ast.Filters.Albums, a)
					albums[k] = true
				}
				known := 0
				for _, a := range ast.Filters.Albums {
					if a.Kind != "name" {
						known++
					}
				}
				if known > 5 {
					return fail(NewDiagnostic("QUERY_TOO_COMPLEX", v.Span, nil))
				}
			case "camera":
				val := Normalize(v.Text)
				ast.Filters.Camera = &val
			case "minsize", "maxsize":
				bytes, ok := parseSize(v.Text)
				if !ok {
					return fail(NewDiagnostic("INVALID_SIZE", v.Span, nil))
				}
				if field == "minsize" {
					ast.Filters.MinBytes = &bytes
				} else {
					ast.Filters.MaxBytes = &bytes
				}
			case "after", "before":
				if !validDate(v.Text) {
					return fail(NewDiagnostic("INVALID_DATE", v.Span, nil))
				}
				val := v.Text
				if field == "after" {
					ast.Filters.After = &val
				} else {
					ast.Filters.Before = &val
				}
			case "visibility":
				val := Normalize(v.Text)
				allowed := []string{"all", "public", "private"}
				if inputField == "is" {
					allowed = []string{"public", "private"}
				}
				if !contains(allowed, val) {
					return fail(NewDiagnostic("INVALID_ENUM", v.Span, map[string]any{"field": inputField}))
				}
				ast.Filters.Visibility = val
			case "sort":
				val := Normalize(v.Text)
				if inputField == "order" {
					mapped, exists := map[string]string{"earliest": "oldest", "utmost": "size-desc", "least": "size-asc", "created_at": "newest", "created_at-asc": "oldest"}[val]
					if !exists {
						return fail(NewDiagnostic("INVALID_ENUM", v.Span, map[string]any{"field": inputField}))
					}
					val = mapped
				}
				if !contains([]string{"newest", "oldest", "size-desc", "size-asc", "name-asc", "name-desc"}, val) {
					return fail(NewDiagnostic("INVALID_ENUM", v.Span, map[string]any{"field": field}))
				}
				ast.Sort = val
			}
		}
	}
	for _, f := range Formats {
		if formats[f] {
			ast.Filters.Formats = append(ast.Filters.Formats, f)
		}
	}
	conflict := func(a, b string) Result {
		x, y := seen[a].Span, seen[b].Span
		return fail(NewDiagnostic("RANGE_CONFLICT", Span{min(x.Start, y.Start), max(x.End, y.End)}, nil))
	}
	if ast.Filters.MinBytes != nil && ast.Filters.MaxBytes != nil {
		a, _ := strconv.ParseInt(*ast.Filters.MinBytes, 10, 64)
		b, _ := strconv.ParseInt(*ast.Filters.MaxBytes, 10, 64)
		if a > b {
			return conflict("minsize", "maxsize")
		}
	}
	if ast.Filters.After != nil && ast.Filters.Before != nil && *ast.Filters.After >= *ast.Filters.Before {
		return conflict("after", "before")
	}
	sortAlbums(ast.Filters.Albums)
	if len(Serialize(ast)) > 4096 {
		return fail(NewDiagnostic("QUERY_TOO_COMPLEX", span(raw, 0, len(raw)), nil))
	}
	return Result{OK: true, AST: &ast, Tokens: tokens}
}
func contains(values []string, s string) bool {
	for _, v := range values {
		if v == s {
			return true
		}
	}
	return false
}

func parseValues(raw string, start, end int, list, plain bool) ([]Value, *Diagnostic) {
	values := []Value{}
	pos := start
	errorAt := func(code string, a, b int) ([]Value, *Diagnostic) {
		d := NewDiagnostic(code, span(raw, a, b), nil)
		return nil, &d
	}
	if pos == end {
		return errorAt("MISSING_VALUE", pos, pos)
	}
	for {
		at := pos
		v := Value{}
		if raw[pos] == '"' {
			v.Quoted = true
			pos++
			valueStart := pos
			var text strings.Builder
			closed := false
			for pos < end {
				r, n := utf8.DecodeRuneInString(raw[pos:end])
				if r == '"' {
					v.Span = span(raw, valueStart, pos)
					pos += n
					closed = true
					break
				}
				if r == '\\' {
					if pos+1 >= end {
						return errorAt("UNCLOSED_QUOTE", at, end)
					}
					next := raw[pos+1]
					if next != '"' && next != '\\' {
						_, sz := utf8.DecodeRuneInString(raw[pos+1 : end])
						return errorAt("INVALID_ESCAPE", pos, pos+1+sz)
					}
					text.WriteByte(next)
					pos += 2
					continue
				}
				text.WriteRune(r)
				pos += n
			}
			if !closed {
				return errorAt("UNCLOSED_QUOTE", at, end)
			}
			v.Text = text.String()
			if pos < end && (!list || raw[pos] != ',') {
				_, sz := utf8.DecodeRuneInString(raw[pos:end])
				return errorAt("UNEXPECTED_CHARACTER", pos, pos+sz)
			}
		} else {
			for pos < end && (!list || raw[pos] != ',') {
				r, n := utf8.DecodeRuneInString(raw[pos:end])
				if r == '"' {
					return errorAt("UNEXPECTED_CHARACTER", pos, pos+n)
				}
				if r == '\\' {
					return errorAt("INVALID_ESCAPE", pos, pos+n)
				}
				if !plain && !list && r == ',' {
					return errorAt("UNEXPECTED_CHARACTER", pos, pos+n)
				}
				pos += n
			}
			v.Text = raw[at:pos]
			v.Span = span(raw, at, pos)
		}
		if v.Text == "" {
			code := "MISSING_VALUE"
			if list && (len(values) > 0 || pos < end) {
				code = "EMPTY_LIST_ITEM"
			}
			return errorAt(code, v.spanByteStart(raw), v.spanByteEnd(raw))
		}
		values = append(values, v)
		if pos == end {
			return values, nil
		}
		pos++
		if pos == end {
			return errorAt("EMPTY_LIST_ITEM", pos, pos)
		}
	}
}

// SpanByte helpers are used only for empty values, whose offsets are equal.
func (v Value) spanByteStart(raw string) int { return byteOffset(raw, v.Span.Start) }
func (v Value) spanByteEnd(raw string) int   { return byteOffset(raw, v.Span.End) }
func byteOffset(s string, u int) int {
	n := 0
	for i, r := range s {
		if n >= u {
			return i
		}
		n++
		if r > 0xffff {
			n++
		}
	}
	return len(s)
}
func parseSize(s string) (string, bool) {
	m := sizeText.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	whole, err := strconv.ParseUint(m[1], 10, 64)
	if err != nil || whole > 1024000 {
		return "", false
	}
	frac := m[2] + strings.Repeat("0", 6-len(m[2]))
	f, _ := strconv.ParseUint(frac, 10, 64)
	bytes := whole*1000000 + f
	if bytes > 1024000000000 {
		return "", false
	}
	return strconv.FormatUint(bytes, 10), true
}
func validDate(s string) bool {
	if !dateText.MatchString(s) || s < "1970-01-01" || s > "9999-12-31" {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}
func quote(s string) string { return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"` }

// Serialize preserves semantic order and produces an idempotent query, never a URL.
func Serialize(ast AST) string {
	out := []string{}
	for _, t := range ast.Terms {
		out = append(out, quote(t))
	}
	if len(ast.Filters.Formats) > 0 {
		ordered := []string{}
		for _, f := range Formats {
			if contains(ast.Filters.Formats, f) {
				ordered = append(ordered, f)
			}
		}
		out = append(out, "format:"+strings.Join(ordered, ","))
	}
	if len(ast.Filters.Albums) > 0 {
		values := []string{}
		as := append([]Album{}, ast.Filters.Albums...)
		sortAlbums(as)
		for _, a := range as {
			switch a.Kind {
			case "name":
				values = append(values, quote(a.Value))
			case "id":
				values = append(values, "#"+a.Value)
			case "unfiled":
				values = append(values, "unfiled")
			}
		}
		out = append(out, "album:"+strings.Join(values, ","))
	}
	if ast.Filters.Camera != nil {
		out = append(out, "camera:"+quote(*ast.Filters.Camera))
	}
	for _, v := range []struct {
		key   string
		value *string
	}{{"minsize", ast.Filters.MinBytes}, {"maxsize", ast.Filters.MaxBytes}} {
		if v.value != nil {
			n, _ := strconv.ParseUint(*v.value, 10, 64)
			value := strconv.FormatUint(n/1000000, 10)
			if n%1000000 != 0 {
				value += "." + strings.TrimRight(fmt.Sprintf("%06d", n%1000000), "0")
			}
			out = append(out, v.key+":"+value+"MB")
		}
	}
	if ast.Filters.After != nil {
		out = append(out, "after:"+*ast.Filters.After)
	}
	if ast.Filters.Before != nil {
		out = append(out, "before:"+*ast.Filters.Before)
	}
	if ast.Filters.Visibility != "all" && ast.Filters.Visibility != "" {
		out = append(out, "visibility:"+ast.Filters.Visibility)
	}
	if ast.Sort != "newest" && ast.Sort != "" {
		out = append(out, "sort:"+ast.Sort)
	}
	return strings.Join(out, " ")
}

func sortAlbums(as []Album) {
	sort.SliceStable(as, func(i, j int) bool {
		rank := func(a Album) int {
			if a.Kind == "unfiled" {
				return 0
			}
			if a.Kind == "id" {
				return 1
			}
			return 2
		}
		a, b := rank(as[i]), rank(as[j])
		if a != b {
			return a < b
		}
		if a != 1 {
			return false
		}
		if len(as[i].Value) != len(as[j].Value) {
			return len(as[i].Value) < len(as[j].Value)
		}
		return as[i].Value < as[j].Value
	})
}

// AlbumValue identifies a validated album token without confusing a quoted
// literal name with the bare ID or unfiled syntax that has the same text.
func AlbumValue(v Value) Album {
	if !v.Quoted {
		if Normalize(v.Text) == "unfiled" {
			return Album{Kind: "unfiled"}
		}
		if idText.MatchString(v.Text) {
			return Album{Kind: "id", Value: v.Text[1:]}
		}
	}
	return Album{Kind: "name", Value: NFC(v.Text)}
}
